package redis

import (
	"github.com/hibiken/asynq"
)

const (
	QueueCritical = "critical"
	QueueDefault  = "default"
	QueueLow      = "low"
)

func NewAsynqClient(host, port, password string, db int) *asynq.Client {
	return asynq.NewClient(asynq.RedisClientOpt{
		Addr:     host + ":" + port,
		Password: password,
		DB:       db,
	})
}

func NewAsynqServer(host, port, password string, db int) *asynq.Server {
	return asynq.NewServer(
		asynq.RedisClientOpt{
			Addr:     host + ":" + port,
			Password: password,
			DB:       db,
		},
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

func NewAsynqScheduler(host, port, password string, db int) *asynq.Scheduler {
	return asynq.NewScheduler(
		asynq.RedisClientOpt{
			Addr:     host + ":" + port,
			Password: password,
			DB:       db,
		},
		nil,
	)
}

