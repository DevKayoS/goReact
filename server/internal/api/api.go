package api

import (
	"context"
	"errors"
	"goReact/internal/store/pgstore"
	"log/slog"
	"net/http"
	"sync"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/go-chi/cors"
	"github.com/google/uuid"
	"github.com/gorilla/websocket"
	"github.com/jackc/pgx/v5"
)

type apiHandler struct {
	q          *pgstore.Queries
	r          *chi.Mux
	upgrader   websocket.Upgrader
	subscriber map[string]map[*websocket.Conn]context.CancelFunc
	mu         *sync.Mutex
}

func (h *apiHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	h.r.ServeHTTP(w, r)
}

func NewHandler(q *pgstore.Queries) http.Handler {
	a := &apiHandler{
		q:          q,
		upgrader:   websocket.Upgrader{CheckOrigin: func(r *http.Request) bool { return true }},
		subscriber: make(map[string]map[*websocket.Conn]context.CancelFunc),
		mu:         &sync.Mutex{},
	}
	r := chi.NewRouter()

	r.Use(
		middleware.RequestID, // todos os requests tem um Id
		middleware.Recoverer, // Quando o codigo gerar um panico nao vai para a execucao do codigo
		middleware.Logger,    // vai gerar logs
	)

	// configurando cors
	r.Use(cors.Handler(cors.Options{
		AllowedOrigins:   []string{"https://*", "http://*"},
		AllowedMethods:   []string{"GET", "POST", "PUT", "DELETE", "OPTIONS", "PATCH"},
		AllowedHeaders:   []string{"Accept", "Authorization", "Content-Type", "X-CSRF-Token"},
		ExposedHeaders:   []string{"Link"},
		AllowCredentials: false,
		MaxAge:           300,
	}))

	r.Get("/subscribe/{room_id}", a.handleSubscribe)

	r.Route("/api", func(r chi.Router) {
		r.Route("/rooms", func(r chi.Router) {
			r.Post("/", a.handleCreateRoom)
			r.Get("/", a.handleGetRooms)

			r.Route("/{room_id}/messages", func(r chi.Router) {
				r.Post("/", a.handleCreateRoomMessage)
				r.Get("/", a.handleGetRoomMessages)

				r.Route("/{message_id}", func(r chi.Router) {
					r.Get("/", a.handleGetRoomMessage)
					r.Patch("/react", a.handleReactToMessage)
					r.Delete("/react", a.handleDeleteReactToMessage)
					r.Patch("/answered", a.handleMarkMessageToAnsewered)
				})
			})
		})
	})

	a.r = r

	return a
}

// WS
func (h apiHandler) handleSubscribe(w http.ResponseWriter, r *http.Request) {
	rawRoomId := chi.URLParam(r, "room_id")
	roomId, err := uuid.Parse(rawRoomId)

	if err != nil {
		http.Error(w, "invalid room id", http.StatusBadRequest)
		return
	}

	_, err = h.q.GetRoom(r.Context(), roomId)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			http.Error(w, "room not found", http.StatusBadRequest)
			return
		}

		http.Error(w, "something went wrong", http.StatusInternalServerError)
		return
	}

	c, err := h.upgrader.Upgrade(w, r, nil)
	if err != nil {
		slog.Warn("failed to upgrade connection", "error", err)
		http.Error(w, "failed to upgrade to ws connection", http.StatusBadRequest)
		return
	}

	defer c.Close()

	_, cancel := context.WithCancel(r.Context())

	h.mu.Lock()
	if _, ok := h.subscriber[rawRoomId]; ok {
		h.subscriber[rawRoomId][c] = cancel
	} else {
		h.subscriber[rawRoomId] = make(map[*websocket.Conn]context.CancelFunc)
		h.subscriber[rawRoomId][c] = cancel
	}
}

// API
func (h apiHandler) handleCreateRoom(w http.ResponseWriter, r *http.Request)             {}
func (h apiHandler) handleGetRooms(w http.ResponseWriter, r *http.Request)               {}
func (h apiHandler) handleGetRoomMessages(w http.ResponseWriter, r *http.Request)        {}
func (h apiHandler) handleCreateRoomMessage(w http.ResponseWriter, r *http.Request)      {}
func (h apiHandler) handleGetRoomMessage(w http.ResponseWriter, r *http.Request)         {}
func (h apiHandler) handleReactToMessage(w http.ResponseWriter, r *http.Request)         {}
func (h apiHandler) handleDeleteReactToMessage(w http.ResponseWriter, r *http.Request)   {}
func (h apiHandler) handleMarkMessageToAnsewered(w http.ResponseWriter, r *http.Request) {}
