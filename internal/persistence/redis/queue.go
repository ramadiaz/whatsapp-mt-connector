package redis

import (
	"github.com/hibiken/asynq"
)

const (
	QueueCritical = "critical"
	QueueDefault  = "default"
	QueueLow      = "low"
)

func NewAsynqClient(redisURL string) *asynq.Client {
	return asynq.NewClient(asynq.RedisClientOpt{Addr: parseRedisAddr(redisURL)})
}

func NewAsynqServer(redisURL string) *asynq.Server {
	return asynq.NewServer(
		asynq.RedisClientOpt{Addr: parseRedisAddr(redisURL)},
		asynq.Config{
			Concurrency: 10,
			Queues: map[string]int{
				QueueCritical: 6,
				QueueDefault:  3,
				QueueLow:      1,
			},
		},
	)
}

func NewAsynqScheduler(redisURL string) *asynq.Scheduler {
	return asynq.NewScheduler(
		asynq.RedisClientOpt{Addr: parseRedisAddr(redisURL)},
		nil,
	)
}

func parseRedisAddr(redisURL string) string {
	if len(redisURL) > 8 && redisURL[:8] == "redis://" {
		return redisURL[8:]
	}
	return redisURL
}
