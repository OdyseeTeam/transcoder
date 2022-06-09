package priority

import (
	"context"
	"errors"

	"github.com/go-redis/redis/v8"
)

type Item struct {
	URL string
}

type Position struct {
	Score float64
	Item  Item
}

type Queue struct {
	key string
	rdb *redis.Client
}

func NewQueue(redisOpts *redis.Options) *Queue {
	rdb := redis.NewClient(redisOpts)
	return &Queue{key: "queue", rdb: rdb}
}

func (q *Queue) Inc(stream Item) error {
	_, err := q.rdb.ZIncrBy(context.Background(), q.key, 1, stream.URL).Result()
	return err
}

func (q *Queue) Pop() (*Position, error) {
	val, err := q.rdb.ZPopMax(context.Background(), q.key, 1).Result()
	if err != nil {
		return nil, err
	}
	if len(val) == 0 {
		return nil, errors.New("zero result")
	}
	s := Item{URL: val[0].Member.(string)}
	return &Position{Item: s, Score: val[0].Score}, nil
}

func (q *Queue) Return(position *Position) error {
	_, err := q.rdb.ZAdd(context.Background(), q.key, &redis.Z{Score: position.Score, Member: position.Item}).Result()
	return err
}
