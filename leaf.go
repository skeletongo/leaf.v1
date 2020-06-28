package leaf

import (
	"os"
	"os/signal"

	"github.com/skeletongo/leaf.v1/cluster"
	"github.com/skeletongo/leaf.v1/conf"
	"github.com/skeletongo/leaf.v1/console"
	"github.com/skeletongo/leaf.v1/log"
	"github.com/skeletongo/leaf.v1/module"
)

func Run(modules ...module.Module) {
	if conf.LogLevel != "" {
		l, err := log.New(conf.LogLevel, conf.LogPath, conf.LogFlag)
		if err != nil {
			panic(err)
		}
		log.Export(l)
		defer l.Close()
	}

	log.Release("Leaf %v starting up", version)

	for i := 0; i < len(modules); i++ {
		module.Register(modules[i])
	}
	module.Init()
	cluster.Init()
	console.Init()

	c := make(chan os.Signal, 1)
	signal.Notify(c, os.Interrupt, os.Kill)
	sig := <-c
	log.Release("Leaf closing down (signal: %v)", sig)

	console.Destroy()
	cluster.Destroy()
	module.Destroy()
}
