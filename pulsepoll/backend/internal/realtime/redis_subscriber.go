package realtime

import (
	"context"
	"encoding/json"
	"github.com/redis/go-redis/v9"
	"log"
)

func StartRedisSubscriber(ctx context.Context, rdb *redis.Client, hub *Hub) {
	go func() {
		pubsub := rdb.PSubscribe(ctx, "poll:*:events")
		defer pubsub.Close()
		for {
			msg, err := pubsub.ReceiveMessage(ctx)
			if err != nil {
				if ctx.Err() != nil {
					return
				}
				log.Printf("redis subscriber: %v", err)
				continue
			}
			var e Event
			if err := json.Unmarshal([]byte(msg.Payload), &e); err != nil {
				log.Printf("redis event decode: %v", err)
				continue
			}
			hub.Broadcast(e)
		}
	}()
}
