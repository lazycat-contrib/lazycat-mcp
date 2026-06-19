package main

import "lazycat-mcp/internal/app"

func main() {
	if err := app.Run(); err != nil {
		panic(err)
	}
}
