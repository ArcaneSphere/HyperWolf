package daemon

import (
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"math"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

type Info struct {
	TopoHeight   int64  `json:"topoheight"`
	StableHeight int64  `json:"stableheight"`
	Difficulty   int64  `json:"difficulty"`
	Version      string `json:"version"`
	Network      string `json:"network"`
	MempoolSize  int    `json:"tx_pool_size"`
}

type SCIDVarData struct {
	SCID          string `json:"scid"`
	DURL          string `json:"dURL"`
	NameHdr       string `json:"nameHdr"`
	DescrHdr      string `json:"descrHdr"`
	IconURL       string `json:"iconURL"`
	Likes         int    `json:"likes"`
	Dislikes      int    `json:"dislikes"`
	Average       int    `json:"average"`
	CreatedHeight int64  `json:"createdHeight"`
}

func GetInfo(node string) *Info {
	return getInfoFromDaemon(node)
}

func getInfoFromDaemon(node string) *Info {
	client := &http.Client{Timeout: 5 * time.Second}
	body := strings.NewReader(`{"jsonrpc":"2.0","id":"1","method":"DERO.GetInfo"}`)
	resp, err := client.Post(node+"/json_rpc", "application/json", body)
	if err != nil {
		log.Printf("getDaemonInfo error: %v", err)
		return nil
	}
	defer resp.Body.Close()

	var raw struct {
		Result map[string]any `json:"result"`
	}
	json.NewDecoder(resp.Body).Decode(&raw)
	if raw.Result == nil {
		return nil
	}
	return parseInfo(raw.Result)
}

func parseInfo(data map[string]any) *Info {
	info := &Info{}
	if h, ok := data["topoheight"].(float64); ok {
		info.TopoHeight = int64(h)
	}
	if h, ok := data["stableheight"].(float64); ok {
		info.StableHeight = int64(h)
	}
	if d, ok := data["difficulty"].(float64); ok {
		info.Difficulty = int64(d)
	}
	if v, ok := data["version"].(string); ok {
		info.Version = v
	}
	if n, ok := data["network"].(string); ok {
		info.Network = n
	}
	if info.Network == "" {
		if testnet, ok := data["testnet"].(bool); ok && testnet {
			info.Network = "Testnet"
		} else {
			info.Network = "Mainnet"
		}
	}
	if m, ok := data["tx_pool_size"].(float64); ok {
		info.MempoolSize = int(m)
	}
	return info
}

func FetchSCIDVariables(node string, scids []string) []SCIDVarData {
	type jobResult struct {
		scid string
		ok   bool
	}
	type scVar struct {
		Key   string `json:"Key"`
		Value string `json:"Value"`
	}
	type daemonResp struct {
		Result *struct {
			StringKeys map[string]any `json:"stringkeys"`
			Uint64Keys map[string]any `json:"uint64keys"`
		} `json:"result"`
		Error *struct {
			Code    int    `json:"code"`
			Message string `json:"message"`
		} `json:"error"`
	}

	const workers = 8
	jobs := make(chan string, len(scids))
	results := make(chan *SCIDVarData, len(scids))
	var wg sync.WaitGroup
	var failCount atomic.Int64

	client := &http.Client{Timeout: 15 * time.Second}

	for range workers {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for scid := range jobs {
				reqBody := fmt.Sprintf(
					`{"jsonrpc":"2.0","id":"1","method":"DERO.GetSC","params":{"scid":%q,"code":false,"variables":true}}`,
					scid,
				)
				resp, err := client.Post(node+"/json_rpc", "application/json", strings.NewReader(reqBody))
				if err != nil {
					failCount.Add(1)
					continue
				}
				body, err := io.ReadAll(resp.Body)
				resp.Body.Close()
				if err != nil {
					failCount.Add(1)
					continue
				}

				var rpc daemonResp
				if err := json.Unmarshal(body, &rpc); err != nil {
					failCount.Add(1)
					continue
				}
				if rpc.Result == nil {
					if rpc.Error != nil {
						log.Printf("fetchSCIDVariables daemon error for %.16s: code=%d msg=%s", scid, rpc.Error.Code, rpc.Error.Message)
					}
					failCount.Add(1)
					continue
				}

				out := &SCIDVarData{SCID: scid}
				ratings := []struct {
					rating float64
					height int64
				}{}
				var aggLikes, aggDislikes int
				var hasAggLikes, hasAggDislikes bool

				for key, raw := range rpc.Result.StringKeys {
					var val string
					switch v := raw.(type) {
					case string:
						val = v
						if d, err := hex.DecodeString(val); err == nil {
							val = string(d)
						}
					case float64:
						val = strconv.Itoa(int(v))
					}

					switch key {
					case "dURL":
						out.DURL = val
					case "nameHdr", "var_header_name":
						out.NameHdr = val
					case "descrHdr", "var_header_description":
						out.DescrHdr = val
					case "iconURLHdr", "var_header_icon":
						out.IconURL = val
					case "likes":
						if n, err := strconv.Atoi(val); err == nil {
							aggLikes = n
							hasAggLikes = true
						}
					case "dislikes":
						if n, err := strconv.Atoi(val); err == nil {
							aggDislikes = n
							hasAggDislikes = true
						}
					default:
						if (strings.HasPrefix(key, "dero1") || strings.HasPrefix(key, "deto1")) && len(key) > 64 {
							parts := strings.SplitN(val, "_", 2)
							if len(parts) == 2 {
								r, _ := strconv.ParseFloat(parts[0], 64)
								h, _ := strconv.ParseInt(parts[1], 10, 64)
								ratings = append(ratings, struct {
									rating float64
									height int64
								}{r, h})
								if h > 0 && (out.CreatedHeight == 0 || h < out.CreatedHeight) {
									out.CreatedHeight = h
								}
							}
						}
					}
				}

				if hasAggLikes {
					out.Likes = aggLikes
				} else {
					for _, r := range ratings {
						if r.rating >= 50 {
							out.Likes++
						}
					}
				}
				if hasAggDislikes {
					out.Dislikes = aggDislikes
				} else {
					for _, r := range ratings {
						if r.rating < 50 {
							out.Dislikes++
						}
					}
				}
				if len(ratings) > 0 {
					var sum float64
					for _, r := range ratings {
						sum += r.rating
					}
					out.Average = int(math.Round(sum / float64(len(ratings))))
				}

				if out.DURL == "" {
					out.DURL = scid
				}
				if out.NameHdr == "" {
					out.NameHdr = scid
				}

				results <- out
			}
		}()
	}

	for _, scid := range scids {
		jobs <- scid
	}
	close(jobs)
	wg.Wait()
	close(results)

	var all []SCIDVarData
	for r := range results {
		all = append(all, *r)
	}
	fails := failCount.Load()
	if fails > 0 {
		log.Printf("fetchSCIDVariables: %d/%d SCIDs failed", fails, len(scids))
	}
	return all
}
