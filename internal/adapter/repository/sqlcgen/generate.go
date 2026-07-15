// Package sqlcgen contains the typed Go code sqlc generates from
// db/queries/*.sql against the schema in db/migrations/*.sql. Do not edit
// any other file in this package by hand: change the SQL, then
// `go generate ./...`.
package sqlcgen

//go:generate go tool sqlc generate -f ../../../../sqlc.yaml
