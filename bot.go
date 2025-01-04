package bot

import (
	"context"
	"fmt"
	"github.com/mikhikhi/baleBot/models"
	"log"

	"net/http"
	"strings"
	"sync"
	"time"
)

const (
	defaultPollTimeout      = time.Minute
	defaultUpdatesChanCap   = 1024
	defaultCheckInitTimeout = time.Second * 5
	defaultWorkers          = 1
)

type HttpClient interface {
	Do(*http.Request) (*http.Response, error)
}

type ErrorsHandler func(err error)
type DebugHandler func(format string, args ...any)
type Middleware func(next HandlerFunc) HandlerFunc
type HandlerFunc func(ctx context.Context, bot *Bot, update *models.Update)
type MatchFunc func(update *models.Update) bool

// Bot represents Telegram Bot main object
type Bot struct {
	lastUpdateID int64

	url                string
	token              string
	pollTimeout        time.Duration
	skipGetMe          bool
	webhookSecretToken string
	testEnvironment    bool
	workers            int
	notAsyncHandlers   bool

	defaultHandlerFunc HandlerFunc

	errorsHandler ErrorsHandler
	debugHandler  DebugHandler

	middlewares []Middleware

	handlersMx sync.RWMutex
	handlers   []handler

	client           HttpClient
	isDebug          bool
	checkInitTimeout time.Duration

	allowedUpdates AllowedUpdates

	updates chan *models.Update
}

// New creates new Bot instance
func New(token string, options ...Option) (*Bot, error) {
	if strings.TrimSpace(token) == "" {
		return nil, fmt.Errorf("empty token")
	}

	b := &Bot{
		url:         "https://tapi.bale.ai",
		token:       token,
		pollTimeout: defaultPollTimeout,
		client: &http.Client{
			Timeout: defaultPollTimeout,
		},
		defaultHandlerFunc: defaultHandler,
		errorsHandler:      defaultErrorsHandler,
		debugHandler:       defaultDebugHandler,
		checkInitTimeout:   defaultCheckInitTimeout,
		workers:            defaultWorkers,

		updates: make(chan *models.Update, defaultUpdatesChanCap),
	}

	for _, o := range options {
		o(b)
	}

	ctx, cancel := context.WithTimeout(context.Background(), b.checkInitTimeout)
	defer cancel()

	if !b.skipGetMe {
		_, err := b.GetMe(ctx)
		if err != nil {
			return nil, fmt.Errorf("error call getMe, %w", err)
		}
	}

	return b, nil
}

// SetToken sets the bot token
func (b *Bot) SetToken(token string) {
	b.token = token
}

// StartWebhook starts the Bot with webhook mode
func (b *Bot) StartWebhook(ctx context.Context) {
	wg := sync.WaitGroup{}

	wg.Add(b.workers)
	for i := 0; i < b.workers; i++ {
		go b.waitUpdates(ctx, &wg)
	}

	wg.Wait()
}

// Start the bot
func (b *Bot) Start(ctx context.Context) {
	wg := sync.WaitGroup{}

	wg.Add(1)
	go b.getUpdates(ctx, &wg)

	wg.Add(b.workers)
	for i := 0; i < b.workers; i++ {
		go b.waitUpdates(ctx, &wg)
	}

	wg.Wait()
}

func defaultErrorsHandler(err error) {
	log.Printf("[TGBOT] [ERROR] %v", err)
}

func defaultDebugHandler(format string, args ...interface{}) {
	log.Printf("[TGBOT] [DEBUG] "+format, args...)
}

func defaultHandler(_ context.Context, _ *Bot, update *models.Update) {
	log.Printf("[TGBOT] [UPDATE] %+v", update)
}

func (b *Bot) error(format string, args ...interface{}) {
	b.errorsHandler(fmt.Errorf(format, args...))
}

func True() *bool {
	b := true
	return &b
}

func False() *bool {
	b := false
	return &b
}

// FileDownloadLink returns the file download link
func (b *Bot) FileDownloadLink(f *models.File) string {
	return fmt.Sprintf("%s/file/bot%s/%s", b.url, b.token, f.FilePath)
}