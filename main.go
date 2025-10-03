package main

import (
	"log"

	"github.com/nixxel-company-limited/escpos-usb-server/adapter"
	"github.com/nixxel-company-limited/escpos-usb-server/server"
	"github.com/spf13/viper"
)

func main() {
	// Initialize Viper to read from environment variables
	viper.AutomaticEnv()
	viper.SetDefault("SERVER_ADDRESS", "localhost:9100")

	// Get server address from environment variable
	address := viper.GetString("SERVER_ADDRESS")
	log.Printf("Server will listen on: %s", address)

	device, err := adapter.NewUSBAdapterAuto()
	if err != nil {
		panic(err)
	}
	defer device.Close()

	svr := server.New(device, address)
	if err := svr.Start(); err != nil {
		panic(err)
	}
}
