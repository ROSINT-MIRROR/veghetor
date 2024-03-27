package main

import (
	"os"

	"veghetor/daemon"

	"github.com/cristalhq/aconfig"
	"github.com/cristalhq/aconfig/aconfigtoml"
	"github.com/cristalhq/aconfig/aconfigyaml"
)

func main() {
	var config daemon.Config
	loader := aconfig.LoaderFor(&config, aconfig.Config{
		EnvPrefix: "VEGHETOR",
		Files:     []string{"config.toml", "config.yaml", "/etc/veghetor.toml", "/etc/veghetor.yaml"},
		FileDecoders: map[string]aconfig.FileDecoder{
			".toml": aconfigtoml.New(),
			".yaml": aconfigyaml.New(),
		},
	})

	flagSet := loader.Flags()

	if err := flagSet.Parse(os.Args[1:]); err != nil {
		os.Exit(1)
	}

	if err := loader.Load(); err != nil {
		os.Exit(1)
	}

	daemon := daemon.NewDaemon(&config)
	defer daemon.Close()

	daemon.Start()
}
