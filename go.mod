module hyperwolf

go 1.26.0

require (
	github.com/civilware/tela v0.0.0-20250806221602-aa892d2ff8d4
	github.com/gogpu/systray v0.1.0
	github.com/gorilla/websocket v1.5.0
	github.com/hypergnomon/hypergnomon v1.1.0
	github.com/sirupsen/logrus v1.9.3
	go.etcd.io/bbolt v1.3.7
)

require (
	github.com/blang/semver/v4 v4.0.0 // indirect
	github.com/caarlos0/env/v6 v6.10.1 // indirect
	github.com/cespare/xxhash v1.1.0 // indirect
	github.com/civilware/Gnomon v0.0.0-20240403103529-8b2fdb2b3106 // indirect
	github.com/coder/websocket v1.8.12 // indirect
	github.com/creachadair/jrpc2 v0.43.0 // indirect
	github.com/davecgh/go-spew v1.1.1 // indirect
	github.com/dchest/siphash v1.2.3 // indirect
	github.com/deroproject/derohe v0.0.0-20240405032004-bd300c0e086e // indirect
	github.com/deroproject/graviton v0.0.0-20220130070622-2c248a53b2e1 // indirect
	github.com/fxamacker/cbor/v2 v2.9.1 // indirect
	github.com/go-logr/logr v1.4.3 // indirect
	github.com/go-logr/zapr v1.3.0 // indirect
	github.com/go-webgpu/goffi v0.5.0 // indirect
	github.com/godbus/dbus/v5 v5.1.0 // indirect
	github.com/gorilla/mux v1.8.1 // indirect
	github.com/klauspost/cpuid/v2 v2.2.6 // indirect
	github.com/minio/sha256-simd v1.0.1 // indirect
	github.com/pkg/errors v0.9.1 // indirect
	github.com/pmezard/go-difflib v1.0.0 // indirect
	github.com/robfig/cron/v3 v3.0.1 // indirect
	github.com/satori/go.uuid v1.2.0 // indirect
	github.com/segmentio/fasthash v1.0.3 // indirect
	github.com/stretchr/testify v1.9.0 // indirect
	github.com/templexxx/xorsimd v0.4.3 // indirect
	github.com/tjfoc/gmsm v1.4.1 // indirect
	github.com/valyala/histogram v1.2.0 // indirect
	github.com/vmihailenco/msgpack/v5 v5.4.1 // indirect
	github.com/vmihailenco/tagparser/v2 v2.0.0 // indirect
	github.com/x448/float16 v0.8.4 // indirect
	go.uber.org/multierr v1.10.0 // indirect
	go.uber.org/zap v1.27.1 // indirect
	golang.org/x/crypto v0.53.0 // indirect
	golang.org/x/net v0.56.0 // indirect
	golang.org/x/sync v0.20.0 // indirect
	golang.org/x/sys v0.46.0 // indirect
	golang.org/x/xerrors v0.0.0-20240903120638-7835f813f4da // indirect
	gopkg.in/yaml.v3 v3.0.1 // indirect
)

replace github.com/deroproject/derohe => github.com/civilware/derohe v0.0.0-20240909003240-fa76d6016cc6

replace github.com/civilware/tela => github.com/Azylem/tela v0.0.0-20260321215354-202c8ca5ce79

// ArcaneSphere/HyperGnomon fork, vendored in-repo (internal/fork/hypergnomon)
// so the CORS middleware patch (api/http.go) survives go clean -modcache.
replace github.com/hypergnomon/hypergnomon => ./internal/fork/hypergnomon
