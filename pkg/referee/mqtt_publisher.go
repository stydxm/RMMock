package referee

import (
	"github.com/sirupsen/logrus"
	"github.com/stydxm/RMMock/pkg/rmcp"
	"google.golang.org/protobuf/proto"
	"time"
)

func Publish() {
	epoch := 0
	initTime := time.Now().Unix()
	for {
		epoch++
		time.Sleep(1000 / 150 * time.Millisecond)
		if epoch%30 == 0 { //5HZ
			out, err := proto.Marshal(&rmcp.GameStatus{
				CurrentRound:      1,
				TotalRounds:       1,
				RedScore:          0,
				BlueScore:         0,
				CurrentStage:      0,
				StageCountdownSec: 0,
				StageElapsedSec:   int32(time.Now().Unix() - initTime),
				IsPaused:          false,
			})
			if err != nil {
				logrus.Warnf("pb序列化失败: %v", err)
			}
			err = server.Publish("GameStatus", out, false, 0)
		}
	}
}
