module github.com/last9/go-agent/examples/gin-full

go 1.21

replace github.com/last9/go-agent => ../..

require (
	github.com/gin-gonic/gin v1.10.0
	github.com/last9/go-agent v0.0.0
	github.com/lib/pq v1.10.9
	github.com/redis/go-redis/v9 v9.7.0
)
