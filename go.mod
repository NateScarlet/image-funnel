module main

go 1.25.0

require (
	github.com/99designs/gqlgen v0.17.86
	github.com/NateScarlet/gqlgen-batching v1.0.1
	github.com/NateScarlet/iso8601 v0.3.2
	github.com/beevik/etree v1.6.0
	github.com/fsnotify/fsnotify v1.9.0
	github.com/go-webauthn/webauthn v0.17.4
	github.com/golang-jwt/jwt/v5 v5.3.1
	github.com/google/uuid v1.6.0
	github.com/gorilla/mux v1.8.1
	github.com/gorilla/websocket v1.5.3
	github.com/mssola/user_agent v0.6.0
	github.com/pelletier/go-toml/v2 v2.4.0
	github.com/rs/cors v1.11.1
	github.com/stretchr/testify v1.11.1
	github.com/vektah/gqlparser/v2 v2.5.31
	go.uber.org/zap v1.27.1
	golang.org/x/image v0.35.0
	golang.org/x/sync v0.20.0
	golang.org/x/sys v0.45.0
)

require (
	github.com/agnivade/levenshtein v1.2.1 // indirect
	github.com/davecgh/go-spew v1.1.1 // indirect
	github.com/fxamacker/cbor/v2 v2.9.2 // indirect
	github.com/go-viper/mapstructure/v2 v2.5.0 // indirect
	github.com/go-webauthn/x v0.2.6 // indirect
	github.com/goccy/go-yaml v1.19.2 // indirect
	github.com/google/go-tpm v0.9.8 // indirect
	github.com/hashicorp/golang-lru/v2 v2.0.7 // indirect
	github.com/philhofer/fwd v1.2.0 // indirect
	github.com/pmezard/go-difflib v1.0.0 // indirect
	github.com/sosodev/duration v1.3.1 // indirect
	github.com/tinylib/msgp v1.6.4 // indirect
	github.com/urfave/cli/v3 v3.6.1 // indirect
	github.com/x448/float16 v0.8.4 // indirect
	go.uber.org/multierr v1.10.0 // indirect
	golang.org/x/crypto v0.52.0 // indirect
	golang.org/x/mod v0.35.0 // indirect
	golang.org/x/text v0.37.0 // indirect
	golang.org/x/tools v0.44.0 // indirect
	gopkg.in/yaml.v3 v3.0.1 // indirect
)

tool (
	github.com/99designs/gqlgen
	main/scripts/generate-handler-root
)
