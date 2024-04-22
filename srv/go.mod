module github.com/abcxyz/abc-updater/srv

go 1.22

toolchain go1.22.1

replace (
	github.com/abcxyz/abc-updater latest => ../ latest
)

require (
	github.com/abcxyz/pkg v1.0.4
	github.com/google/go-cmp v0.6.0
)

require (
	github.com/golang/protobuf v1.5.3 // indirect
	github.com/google/renameio v1.0.1 // indirect
	github.com/hashicorp/go-version v1.6.0 // indirect
	github.com/sethvargo/go-envconfig v1.0.0 // indirect
	golang.org/x/net v0.22.0 // indirect
	golang.org/x/sys v0.18.0 // indirect
	golang.org/x/text v0.14.0 // indirect
	google.golang.org/genproto/googleapis/rpc v0.0.0-20240304212257-790db918fca8 // indirect
	google.golang.org/grpc v1.62.1 // indirect
	google.golang.org/protobuf v1.33.0 // indirect
)
