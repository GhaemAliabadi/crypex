package hitbtc

import (
	"context"
	"encoding/json"
	"sync"

	"github.com/gorilla/websocket"
	JsonRPC2 "github.com/sourcegraph/jsonrpc2"
	JsonRPC2WS "github.com/sourcegraph/jsonrpc2/websocket"
)

const Endpoint = "wss://api.hitbtc.com/api/2/ws"

type Feeds struct {
	ErrorFeed chan error

	Notifications Notifications

	OrderbookFeed sync.Map
	CandlesFeed   sync.Map
	ReportsFeed   chan ReportsSnapshot
}

type Notifications struct {
	OrderbookFeed sync.Map
	CandlesFeed   sync.Map
	ReportsFeed   chan ReportsUpdate
}

func (r *Feeds) Handle(_ context.Context, _ *JsonRPC2.Conn, request *JsonRPC2.Request) {
	if request.Params == nil {
		return
	}

	message := *request.Params

	switch request.Method {
	case "activeOrders":
		var msg ReportsSnapshot

		err := json.Unmarshal(message, &msg)
		if err != nil {
			r.ErrorFeed <- err
		} else {
			r.ReportsFeed <- msg
		}
	case "report":
		var msg ReportsUpdate

		err := json.Unmarshal(message, &msg)
		if err != nil {
			r.ErrorFeed <- err
		} else {
			r.Notifications.ReportsFeed <- msg
		}
	case "snapshotCandles":
		var msg CandlesSnapshot

		err := json.Unmarshal(message, &msg)
		if err != nil {
			r.ErrorFeed <- err
		} else {
			snapshot, _ := r.CandlesFeed.Load(msg.Symbol)
			snapshot.(chan CandlesSnapshot) <- msg
		}
	case "updateCandles":
		var msg CandlesUpdate

		err := json.Unmarshal(message, &msg)
		if err != nil {
			r.ErrorFeed <- err
		} else {
			update, _ := r.Notifications.CandlesFeed.Load(msg.Symbol)
			update.(chan CandlesUpdate) <- msg
		}
	case "snapshotOrderbook":
		var msg OrderbookSnapshot

		err := json.Unmarshal(message, &msg)
		if err != nil {
			r.ErrorFeed <- err
		} else {
			snapshot, _ := r.OrderbookFeed.Load(msg.Symbol)
			snapshot.(chan OrderbookSnapshot) <- msg
		}
	case "updateOrderbook":
		var msg OrderbookUpdate
		err := json.Unmarshal(message, &msg)

		if err != nil {
			r.ErrorFeed <- err
		} else {
			update, _ := r.Notifications.OrderbookFeed.Load(msg.Symbol)
			update.(chan OrderbookUpdate) <- msg
		}
	}
}

type HitBTC struct {
	Feeds *Feeds

	Conn *JsonRPC2.Conn
}

func New() (instance *HitBTC, err error) {
	connection, _, err := websocket.DefaultDialer.Dial(Endpoint, nil)
	if err != nil {
		return nil, err
	}

	feeds := &Feeds{
		ErrorFeed: make(chan error),

		Notifications: Notifications{
			OrderbookFeed: sync.Map{},
			CandlesFeed:   sync.Map{},
			ReportsFeed:   make(chan ReportsUpdate),
		},

		OrderbookFeed: sync.Map{},
		CandlesFeed:   sync.Map{},
		ReportsFeed:   make(chan ReportsSnapshot),
	}

	instance = &HitBTC{
		Conn: JsonRPC2.NewConn(
			context.Background(),
			JsonRPC2WS.NewObjectStream(connection), JsonRPC2.AsyncHandler(feeds),
		),
		Feeds: feeds,
	}

	return
}

func (h *HitBTC) Authenticate(public, secret string) error {
	request := struct {
		PublicKey string `json:"pKey,required"`
		SecretKey string `json:"sKey,required"`
		Algorithm string `json:"algo,required"`
	}{
		PublicKey: public,
		SecretKey: secret,
		Algorithm: "BASIC",
	}

	err := h.Request("login", &request, nil)
	if err != nil {
		return err
	}

	return nil
}

func (h *HitBTC) Close() {
	_ = h.Conn.Close()

	close(h.Feeds.ErrorFeed)
	close(h.Feeds.ReportsFeed)
	close(h.Feeds.Notifications.ReportsFeed)

	h.Feeds.CandlesFeed.Range(func(key, value interface{}) bool {
		close(value.(chan CandlesSnapshot))

		return true
	})

	h.Feeds.Notifications.CandlesFeed.Range(func(key, value interface{}) bool {
		close(value.(chan CandlesUpdate))

		return true
	})

	h.Feeds.OrderbookFeed.Range(func(key, value interface{}) bool {
		close(value.(chan OrderbookSnapshot))

		return true
	})

	h.Feeds.Notifications.OrderbookFeed.Range(func(key, value interface{}) bool {
		close(value.(chan OrderbookUpdate))

		return true
	})

	h.Feeds.ErrorFeed = make(chan error)
	h.Feeds.ReportsFeed = make(chan ReportsSnapshot)
	h.Feeds.Notifications.ReportsFeed = make(chan ReportsUpdate)

	h.Feeds.CandlesFeed = sync.Map{}
	h.Feeds.Notifications.CandlesFeed = sync.Map{}

	h.Feeds.OrderbookFeed = sync.Map{}
	h.Feeds.Notifications.OrderbookFeed = sync.Map{}
}

type Response bool

func (h *HitBTC) Subscribe(method string, request interface{}) error {
	var response Response

	err := h.Conn.Call(context.Background(), method, &request, &response)
	if err != nil {
		return err
	}

	return nil
}

func (h *HitBTC) Request(method string, request interface{}, response interface{}) error {
	err := h.Conn.Call(context.Background(), method, &request, &response)
	if err != nil {
		return err
	}

	return nil
}
