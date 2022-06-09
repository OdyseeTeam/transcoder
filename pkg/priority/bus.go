package priority

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/go-redis/redis/v8"
)

type TaskReport struct {
	ULID       string
	Status     string
	Progress   float32
	ReceivedAt time.Time
}

type TaskRunner struct {
	rdb *redis.Client
}

type TaskManager struct {
	rdb         *redis.Client
	assignments map[string][]string
	reports     map[string]TaskReport
}

func (m *TaskManager) LoadTasks() error {
	n, err := m.rdb.Del(context.Background(), "inbox:reports:*", "outbox:reports:*").Result()
	if err != nil {
		return err
	}
	fmt.Println(n, " messages discarded")
	return nil
}

func (m *TaskManager) requestTaskReports() {
	timedOut := []string{}
	for wid, ts := range m.assignments {
		rLen, err := m.rdb.LLen(context.Background(), fmt.Sprintf("outbox:reports:%s", wid)).Result()
		if err != nil {
			// log error
			continue
		}
		if rLen > 3 {
			timedOut = append(timedOut, wid)
		}
		for _, t := range ts {
			m.rdb.RPush(context.Background(), fmt.Sprintf("outbox:reports:%s", wid), t)
		}
	}
	for _, wid := range timedOut {
		delete(m.assignments, wid)
	}
}

func (m *TaskManager) receiveTaskReports() {
	for wid, workerTasks := range m.assignments {
		rrep, err := m.rdb.RPop(context.Background(), fmt.Sprintf("inbox:reports:%s", wid)).Result()
		if err != nil {
			// log error
			continue
		}
		repParts := strings.SplitN(rrep, "|", 3)
		progress, _ := strconv.ParseFloat(repParts[3], 32)
		if !Contains(workerTasks, repParts[0]) {
			continue
		}
		m.reports[repParts[0]] = TaskReport{
			ULID:     repParts[0],
			Status:   repParts[1],
			Progress: float32(progress),
		}
	}
}

func (r *TaskRunner) GetInitialTasks()

func Contains(sl []string, item string) bool {
	for _, value := range sl {
		if value == item {
			return true
		}
	}
	return false
}
