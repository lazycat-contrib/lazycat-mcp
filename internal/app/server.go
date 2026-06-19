package app

import (
	"context"
	"errors"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	gohelper "gitee.com/linakesi/lzc-sdk/lang/go"
	mcpserver "github.com/mark3labs/mcp-go/server"

	"lazycat-mcp/ent"
	"lazycat-mcp/internal/pkg/kit"
	"lazycat-mcp/internal/pkg/zlog"
	"lazycat-mcp/internal/proxy"
	"lazycat-mcp/internal/web"
)

type App struct {
	cfg       Config
	logger    *zlog.Logger
	db        *ent.Client
	gateway   *gohelper.APIGateway
	kit       *kit.Manager
	tokens    *TokenService
	providers *ProviderService
	resources *ResourceScanner
	tickets   *TicketStore

	mcpHTTP       http.Handler
	mcpSSE        http.Handler
	ui            http.Handler
	providerProxy http.Handler
}

func New(ctx context.Context, cfg Config, logger *zlog.Logger) (*App, error) {
	db, err := openDB(ctx, cfg.DBPath)
	if err != nil {
		return nil, err
	}

	gw, err := gohelper.NewAPIGateway(ctx)
	if err != nil {
		logger.Warn().Err(err).Msg("lazycat api gateway unavailable")
	}

	providers := NewProviderService(db)
	tickets := &TicketStore{}
	app := &App{
		cfg:       cfg,
		logger:    logger,
		db:        db,
		gateway:   gw,
		kit:       kit.NewManagerWithGateway(gw, logger),
		tokens:    NewTokenService(db),
		providers: providers,
		resources: NewResourceScanner(cfg.ResourceRoot),
		tickets:   tickets,
	}

	mcpServer := app.newMCPServer()
	app.mcpHTTP = mcpserver.NewStreamableHTTPServer(mcpServer)
	app.mcpSSE = mcpserver.NewSSEServer(mcpServer)
	app.ui = web.Console()
	app.providerProxy = proxy.New(providers, tickets)
	return app, nil
}

func Run() error {
	cfg := LoadConfig()
	logger := zlog.NewLogger(zlog.LogConfig{
		LogLevel:    getenv("LAZYCAT_MCP_LOG_LEVEL", "info"),
		LogDir:      getenv("LAZYCAT_MCP_LOG_DIR", "/lzcapp/var/logs"),
		LogFileName: "mcp-app.log",
		MaxSize:     10,
		MaxBackups:  5,
		MaxAge:      7,
	})

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	app, err := New(ctx, cfg, logger)
	if err != nil {
		return err
	}
	defer app.Close()

	srv := &http.Server{
		Addr:              cfg.Addr,
		Handler:           app,
		ReadHeaderTimeout: 10 * time.Second,
	}

	errCh := make(chan error, 1)
	go func() {
		logger.Info().Str("addr", cfg.Addr).Msg("lazycat mcp server listening")
		errCh <- srv.ListenAndServe()
	}()

	select {
	case <-ctx.Done():
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		return srv.Shutdown(shutdownCtx)
	case err := <-errCh:
		if errors.Is(err, http.ErrServerClosed) {
			return nil
		}
		return err
	}
}

func (a *App) Close() {
	if a.gateway != nil {
		_ = a.gateway.Close()
	}
	if a.db != nil {
		_ = a.db.Close()
	}
}

func (a *App) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	setSecurityHeaders(w)

	switch {
	case strings.HasPrefix(r.URL.Path, "/mcp/apps/"):
		a.requireMCPToken(a.providerProxy).ServeHTTP(w, r)
	case r.URL.Path == "/mcp" || r.URL.Path == "/mcp/":
		a.requireMCPToken(a.mcpHTTP).ServeHTTP(w, r)
	case r.URL.Path == "/sse" || r.URL.Path == "/message":
		a.requireMCPToken(a.mcpSSE).ServeHTTP(w, r)
	case strings.HasPrefix(r.URL.Path, "/skills/"):
		a.serveSkill(w, r)
	case r.URL.Path == "/" || r.URL.Path == "/console.css":
		a.tickets.Capture(r)
		a.serveUI(w, r)
	case strings.HasPrefix(r.URL.Path, "/api/"):
		a.tickets.Capture(r)
		a.handleAPI(w, r)
	default:
		http.NotFound(w, r)
	}
}

func (a *App) requireMCPToken(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		token := tokenFromRequest(r)
		if err := a.tokens.Validate(r.Context(), token); err != nil {
			w.Header().Set("WWW-Authenticate", `Bearer realm="lazycat-mcp"`)
			writeAPIError(w, http.StatusUnauthorized, err.Error())
			return
		}
		next.ServeHTTP(w, r)
	})
}

func tokenFromRequest(r *http.Request) string {
	auth := strings.TrimSpace(r.Header.Get("Authorization"))
	if strings.HasPrefix(strings.ToLower(auth), "bearer ") {
		return strings.TrimSpace(auth[7:])
	}
	return strings.TrimSpace(r.Header.Get("X-MCP-Token"))
}

func setSecurityHeaders(w http.ResponseWriter) {
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.Header().Set("Referrer-Policy", "same-origin")
	w.Header().Set("X-Frame-Options", "SAMEORIGIN")
}

func (a *App) serveUI(w http.ResponseWriter, r *http.Request) {
	a.ui.ServeHTTP(w, r)
}

func (a *App) serveSkill(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet && r.Method != http.MethodHead {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	rel := strings.TrimPrefix(r.URL.Path, "/skills/")
	if rel == "" || strings.Contains(rel, "..") || strings.HasPrefix(rel, "/") || strings.HasSuffix(rel, "/") {
		http.NotFound(w, r)
		return
	}
	for _, root := range skillRoots(a.cfg.ResourceRoot) {
		candidate := root + "/" + rel
		if info, err := os.Stat(candidate); err == nil && !info.IsDir() {
			http.ServeFile(w, r, candidate)
			return
		}
	}
	http.NotFound(w, r)
}
