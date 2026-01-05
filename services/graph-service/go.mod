module github.com/jupiterclapton/cenackle/services/graph-service

go 1.24.11

require (
	github.com/jupiterclapton/cenackle/gen v0.0.0-00010101000000-000000000000
	github.com/neo4j/neo4j-go-driver/v5 v5.28.4
	google.golang.org/grpc v1.78.0
	google.golang.org/protobuf v1.36.11
)

require (
	go.opentelemetry.io/otel/metric v1.39.0 // indirect
	go.opentelemetry.io/otel/sdk v1.39.0 // indirect
	go.opentelemetry.io/otel/trace v1.39.0 // indirect
	golang.org/x/net v0.48.0 // indirect
	golang.org/x/sys v0.39.0 // indirect
	golang.org/x/text v0.32.0 // indirect
	google.golang.org/genproto/googleapis/rpc v0.0.0-20251202230838-ff82c1b0f217 // indirect
)

replace github.com/jupiterclapton/cenackle/gen => ../../gen
