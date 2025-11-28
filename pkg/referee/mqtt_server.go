package referee

import (
	"github.com/sirupsen/logrus"
	"log"
	"log/slog"
	"os"
	"os/signal"
	"sync"
	"syscall"

	mqtt "github.com/mochi-mqtt/server/v2"
	"github.com/mochi-mqtt/server/v2/hooks/auth"
	"github.com/mochi-mqtt/server/v2/listeners"
	sloglogrus "github.com/samber/slog-logrus/v2"
)

var server *mqtt.Server

func StartMqttServer(wg *sync.WaitGroup) {
	defer wg.Done()

	sigs := make(chan os.Signal, 1)
	done := make(chan bool, 1)
	signal.Notify(sigs, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		<-sigs
		done <- true
	}()

	slogLogger := slog.New(sloglogrus.Option{Logger: logrus.New()}.NewLogrusHandler())
	server = mqtt.New(&mqtt.Options{Logger: slogLogger, InlineClient: true})
	_ = server.AddHook(new(auth.AllowHook), nil)
	tcp := listeners.NewTCP(listeners.Config{ID: "t1", Address: ":3333"})
	err := server.AddListener(tcp)
	if err != nil {
		log.Fatal(err)
	}

	go func() {
		err := server.Serve()
		if err != nil {
			log.Fatal(err)
		}
	}()

	<-done
}
