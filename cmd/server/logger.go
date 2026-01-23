package main

import (
	"go.uber.org/zap"
)

func initLogger(version string) (*zap.Logger, error) {
	if version != "dev" {
		return zap.NewProduction()
	}
	return zap.NewDevelopment()
}
