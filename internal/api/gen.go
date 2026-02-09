//go:generate go tool oapi-codegen -generate types,server -package api -o api.gen.go ../../api/openapi.yaml
//go:generate mockgen -source=api.gen.go -destination=server.gen.go -package=api

package api
