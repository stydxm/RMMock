package main

import (
	"github.com/sirupsen/logrus"
	"github.com/stydxm/RMMock/pkg/referee"
	"github.com/stydxm/RMMock/pkg/stream"
	"os"
	"sync"
)

func main() {
	logrus.SetOutput(os.Stdout)
	if os.Getenv("mode") == "dev" {
		logrus.SetLevel(logrus.DebugLevel)
	}
	var wg sync.WaitGroup

	streamSource := 0
	wg.Add(1)
	go stream.StartEncodedStream(streamSource, &wg)
	go referee.StartMqttServer(&wg)
	go referee.Publish()

	wg.Wait()
	logrus.Infof("退出程序")
}
