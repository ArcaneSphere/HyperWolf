package explorer

import (
	"encoding/hex"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/deroproject/derohe/block"
	"github.com/deroproject/derohe/cryptography/crypto"
	"github.com/deroproject/derohe/rpc"
	"github.com/deroproject/derohe/transaction"
)

// Block assembles a block header with its decoded miner tx and normal txs.
type Block struct {
	Header    *Header `json:"header"`
	MinerTx   *Tx     `json:"miner_tx,omitempty"`
	Txs       []*Tx   `json:"txs,omitempty"`
	Fees      string  `json:"fees"`
	FeesUint  uint64  `json:"fees_uint64"`
	Size      string  `json:"size"`
	SizeBytes uint64  `json:"size_bytes"`
	TxCount   int     `json:"tx_count"` // includes the coinbase tx
}

// Asset describes one payload of a transaction.
type Asset struct {
	SCID     string `json:"scid"`
	Fees     string `json:"fees"`
	FeesUint uint64 `json:"fees_uint64"`
	Burn     string `json:"burn"`
	BurnUint uint64 `json:"burn_uint64"`
	RingSize int    `json:"ring_size"`
}

// Tx is the presentation form of a single transaction.
type Tx struct {
	Hash          string     `json:"hash"`
	Hex           string     `json:"hex,omitempty"`
	Type          string     `json:"type"`
	Version       uint64     `json:"version"`
	Height        int64      `json:"height"` // mined block height; 0 while in pool
	InPool        bool       `json:"in_pool"`
	Fee           string     `json:"fee,omitempty"`
	FeeUint64     uint64     `json:"fee_uint64,omitempty"`
	BurnValue     string     `json:"burn_value,omitempty"`
	Value         uint64     `json:"value"`
	RingSize      int        `json:"ring_size,omitempty"`
	Ring          [][]string `json:"ring,omitempty"`
	Signer        string     `json:"signer,omitempty"`
	MinerAddress  string     `json:"miner_address,omitempty"`
	Amount        string     `json:"amount,omitempty"`
	ValidBlock    string     `json:"valid_block,omitempty"`
	InvalidBlock  []string   `json:"invalid_block,omitempty"`
	OutputIndices []uint64   `json:"output_indices,omitempty"`
	BLID          string     `json:"blid,omitempty"`
	HeightBuilt   uint64     `json:"height_built,omitempty"`
	SCID          string     `json:"scid,omitempty"`
	SCArgs        []any      `json:"sc_args,omitempty"`
	SCBalance     uint64     `json:"sc_balance,omitempty"`
	SCCode        string     `json:"sc_code,omitempty"`
	RootHash      string     `json:"root_hash,omitempty"`
	Size          string     `json:"size,omitempty"`
	SizeBytes     uint64     `json:"size_bytes"`
	BlockTime     string     `json:"block_time,omitempty"`
	Age           string     `json:"age,omitempty"`
	Depth         int64      `json:"depth,omitempty"`
	Payloads      []Asset    `json:"payloads,omitempty"`
}

func (c *Client) buildBlock(raw *rawBlockResult) (*Block, error) {
	blob, err := hex.DecodeString(raw.Blob)
	if err != nil {
		return nil, fmt.Errorf("decode block blob: %w", err)
	}
	var bl block.Block
	if err := bl.Deserialize(blob); err != nil {
		return nil, fmt.Errorf("deserialize block: %w", err)
	}

	out := &Block{
		Header:    raw.BlockHeader.public(),
		SizeBytes: uint64(len(blob)),
		Size:      fmt.Sprintf("%.03f", float64(len(blob))/1024),
		TxCount:   1 + len(bl.Tx_hashes),
	}

	miner := &Tx{
		Hash:       bl.Miner_TX.GetHash().String(),
		Height:     raw.BlockHeader.Height,
		ValidBlock: raw.BlockHeader.Hash,
		Version:    bl.Miner_TX.Version,
		Type:       bl.Miner_TX.TransactionType.String(),
		Amount:     FormatMoney(bl.Miner_TX.Value + raw.BlockHeader.Reward),
	}
	miner.SizeBytes = uint64(len(bl.Miner_TX.Serialize()))
	miner.Size = fmt.Sprintf("%.03f", float64(miner.SizeBytes)/1024)
	miner.Hex = hex.EncodeToString(bl.Miner_TX.Serialize())
	fillMiner(miner, &bl.Miner_TX)
	bt := time.UnixMilli(int64(raw.BlockHeader.Timestamp))
	miner.BlockTime = bt.UTC().Format("2006-01-02 15:04:05")
	miner.Age = humanizeAge(bt)
	miner.Depth = raw.BlockHeader.Depth
	out.MinerTx = miner

	hashes := make([]string, 0, len(bl.Tx_hashes))
	for i := range bl.Tx_hashes {
		hashes = append(hashes, bl.Tx_hashes[i].String())
	}

	txs, err := c.rawTxs(hashes)
	if err == nil {
		for _, tx := range txs {
			if tx == nil {
				continue
			}
			tx.BlockTime = bt.UTC().Format("2006-01-02 15:04:05")
			tx.Age = humanizeAge(bt)
			tx.Depth = raw.BlockHeader.Depth
			out.FeesUint += tx.FeeUint64
			out.Txs = append(out.Txs, tx)
		}
	}
	out.Fees = FormatMoney(out.FeesUint)
	return out, nil
}

func fillMiner(tx *Tx, dt *transaction.Transaction) {
	tx.RootHash = ""
	if len(dt.Payloads) >= 1 {
		tx.RootHash = fmt.Sprintf("%x", dt.Payloads[0].Statement.Roothash[:])
	}
	switch dt.TransactionType {
	case transaction.PREMINE, transaction.REGISTRATION, transaction.COINBASE:
		if addr := decodeMinerAddress(dt.MinerAddress[:]); addr != "" {
			tx.MinerAddress = addr
		}
	case transaction.NORMAL, transaction.BURN_TX, transaction.SC_TX:
		tx.FeeUint64 = dt.Fees()
		tx.Fee = FormatMoney(dt.Fees())
	}
	if dt.TransactionType == transaction.BURN_TX {
		tx.BurnValue = FormatMoney(dt.Value)
	}
}

func decodeMinerAddress(compressed []byte) string {
	var acckey crypto.Point
	if err := acckey.DecodeCompressed(compressed); err != nil {
		return ""
	}
	return rpc.NewAddressFromKeys(&acckey).String()
}

func (c *Client) rawTxs(hashes []string) ([]*Tx, error) {
	if len(hashes) == 0 {
		return nil, nil
	}
	var out rawTxResult
	if err := c.rpc("DERO.GetTransaction", map[string]any{"txs_hashes": hashes}, &out); err != nil {
		return nil, err
	}
	result := make([]*Tx, 0, len(out.Txs))
	for i := range out.Txs {
		raw := out.Txs[i]
		// Some frontends return tx entries without tx_hash; identify them by
		// position. But derod also answers unknown/pruned hashes with an
		// all-empty entry + status OK, so a back-filled hash alone must not
		// count as content — otherwise every unknown 64-hex (incl. SCIDs)
		// decodes into a phantom empty tx.
		backfilled := len(out.Txs) == len(hashes) && raw.TxHash == ""
		if backfilled {
			raw.TxHash = hashes[i]
		}
		if !txHasContent(raw, backfilled) {
			continue
		}
		tx, err := decodeTx(raw)
		if err != nil {
			continue
		}
		result = append(result, tx)
	}
	return result, nil
}

// txHasContent reports whether the node returned any identifying data for a tx
// entry (pool state, block, signer, hex, SC code, ring set, indices…). An entry
// where nothing was returned except an optionally back-filled queried hash is a
// lookup miss and must be skipped.
func txHasContent(raw rawTxInfo, backfilled bool) bool {
	return raw.ValidBlock != "" || raw.InPool || raw.Signer != "" || raw.AsHex != "" ||
		raw.Code != "" || raw.CodeNow != "" || len(raw.Ring) > 0 ||
		len(raw.OutputIndices) > 0 || raw.BlockHeight != 0 || raw.Reward != 0 ||
		raw.Balance != 0 || raw.BalanceNow != 0 || (!backfilled && raw.TxHash != "")
}

func decodeTx(raw rawTxInfo) (*Tx, error) {
	if raw.TxHash == "" && raw.ValidBlock == "" && !raw.InPool && raw.Signer == "" && raw.AsHex == "" && raw.Code == "" {
		return nil, fmt.Errorf("empty tx")
	}
	tx := &Tx{
		Hash:          raw.TxHash,
		Hex:           raw.AsHex,
		Height:        raw.BlockHeight,
		InPool:        raw.InPool || raw.BlockHeight < 0,
		Ring:          raw.Ring,
		Signer:        raw.Signer,
		SCBalance:     raw.Balance,
		SCCode:        raw.Code,
		ValidBlock:    raw.ValidBlock,
		InvalidBlock:  raw.InvalidBlock,
		OutputIndices: raw.OutputIndices,
		SizeBytes:     uint64(len(raw.AsHex) / 2),
		Size:          fmt.Sprintf("%.03f", float64(len(raw.AsHex)/2)/1024),
	}
	if tx.InPool {
		tx.Height = 0
	}
	if len(raw.Ring) > 0 {
		tx.RingSize = len(raw.Ring[0])
	}
	// Some frontends (e.g. pruned integration nodes) return ring/signer/code
	// but no raw hex; infer the type from the SC code when present.
	if len(raw.AsHex) < 50 {
		if raw.Code != "" {
			tx.Type = "SC"
			if tx.SCID == "" {
				tx.SCID = tx.Hash
			}
		}
		return tx, nil
	}
	bin, err := hex.DecodeString(raw.AsHex)
	if err != nil {
		return tx, nil
	}
	var dt transaction.Transaction
	if err := dt.Deserialize(bin); err != nil {
		return tx, nil
	}

	tx.Type = dt.TransactionType.String()
	tx.Version = dt.Version
	tx.BLID = fmt.Sprintf("%x", dt.BLID)
	tx.HeightBuilt = dt.Height
	tx.Value = dt.Value

	if len(dt.Payloads) >= 1 {
		tx.RootHash = fmt.Sprintf("%x", dt.Payloads[0].Statement.Roothash[:])
		tx.RingSize = int(dt.Payloads[0].Statement.RingSize)
	}

	switch dt.TransactionType {
	case transaction.PREMINE, transaction.REGISTRATION, transaction.COINBASE:
		tx.MinerAddress = decodeMinerAddress(dt.MinerAddress[:])
		if dt.TransactionType == transaction.PREMINE {
			tx.Amount = FormatMoney(dt.Value)
		}
	case transaction.NORMAL, transaction.BURN_TX, transaction.SC_TX:
		tx.FeeUint64 = dt.Fees()
		tx.Fee = FormatMoney(dt.Fees())
		if dt.TransactionType == transaction.BURN_TX {
			tx.BurnValue = FormatMoney(dt.Value)
		}
	}

	if dt.TransactionType == transaction.SC_TX {
		if raw, err := json.Marshal(dt.SCDATA); err == nil {
			_ = json.Unmarshal(raw, &tx.SCArgs)
		}
		tx.SCID = ""
		if len(dt.Payloads) >= 1 {
			scid := dt.Payloads[0].SCID.String()
			if !isZeroHash(scid) {
				tx.SCID = strings.ToLower(scid)
			}
		}
		if tx.SCID == "" {
			tx.SCID = tx.Hash
		}
	}

	for _, p := range dt.Payloads {
		tx.Payloads = append(tx.Payloads, Asset{
			SCID:     strings.ToLower(p.SCID.String()),
			FeesUint: p.Statement.Fees,
			Fees:     FormatMoney(p.Statement.Fees),
			BurnUint: p.BurnValue,
			Burn:     FormatMoney(p.BurnValue),
			RingSize: int(p.Statement.RingSize),
		})
	}
	return tx, nil
}

func isZeroHash(s string) bool {
	for i := 0; i < len(s); i++ {
		if s[i] != '0' {
			return false
		}
	}
	return true
}

func humanizeAge(t time.Time) string {
	d := time.Since(t)
	if d < time.Second {
		return "just now"
	}
	if d < time.Minute {
		return fmt.Sprintf("%ds ago", int(d.Seconds()))
	}
	if d < time.Hour {
		return fmt.Sprintf("%dm %ds ago", int(d.Minutes()), int(d.Seconds())%60)
	}
	if d < 24*time.Hour {
		return fmt.Sprintf("%dh %dm ago", int(d.Hours()), int(d.Minutes())%60)
	}
	return fmt.Sprintf("%dd ago", int(d.Hours())/24)
}
