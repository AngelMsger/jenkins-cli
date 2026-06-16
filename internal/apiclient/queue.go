package apiclient

import (
	"context"
	"net/url"
	"strconv"
	"time"

	"github.com/angelmsger/jenkins-cli/internal/timeutil"
)

type rawQueueItem struct {
	ID           int    `json:"id"`
	Why          string `json:"why"`
	Blocked      bool   `json:"blocked"`
	Buildable    bool   `json:"buildable"`
	Stuck        bool   `json:"stuck"`
	Pending      bool   `json:"pending"`
	InQueueSince int64  `json:"inQueueSince"`
	Params       string `json:"params"`
	URL          string `json:"url"`
	Task         struct {
		Name string `json:"name"`
		URL  string `json:"url"`
	} `json:"task"`
}

const queueItemTree = "id,why,blocked,buildable,stuck,pending,inQueueSince,params,url,task[name,url]"

// ListQueue returns the build queue (pending / blocked builds).
func (c *apiClient) ListQueue(ctx context.Context) ([]QueueItem, error) {
	q := url.Values{"tree": {"items[" + queueItemTree + "]"}}
	var raw struct {
		Items []rawQueueItem `json:"items"`
	}
	if err := c.getJSON(ctx, "/queue/api/json", q, &raw); err != nil {
		return nil, err
	}
	out := make([]QueueItem, 0, len(raw.Items))
	for _, it := range raw.Items {
		out = append(out, toQueueItem(it))
	}
	return out, nil
}

// GetQueueItem returns one queue item by id.
func (c *apiClient) GetQueueItem(ctx context.Context, id int) (*QueueItem, error) {
	q := url.Values{"tree": {queueItemTree}}
	var raw rawQueueItem
	if err := c.getJSON(ctx, "/queue/item/"+strconv.Itoa(id)+"/api/json", q, &raw); err != nil {
		return nil, err
	}
	it := toQueueItem(raw)
	return &it, nil
}

func toQueueItem(r rawQueueItem) QueueItem {
	it := QueueItem{
		ID:        r.ID,
		Why:       r.Why,
		Task:      r.Task.Name,
		URL:       r.Task.URL,
		Blocked:   r.Blocked,
		Buildable: r.Buildable,
		Stuck:     r.Stuck,
		Pending:   r.Pending,
		Params:    r.Params,
	}
	if r.InQueueSince > 0 {
		it.InQueueSince = timeutil.FromMillis(r.InQueueSince).Format(time.RFC3339)
	}
	return it
}
