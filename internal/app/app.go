package app

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"encoding/json"
	"os"
	"os/exec"
	"os/signal"
	"runtime"
	"strings"
	"syscall"
	"time"

	"github.com/Mag1cFall/AIStudio2API/internal/api"
	"github.com/Mag1cFall/AIStudio2API/internal/config"
	"github.com/Mag1cFall/AIStudio2API/internal/setup"
	"github.com/Mag1cFall/AIStudio2API/internal/webui"
)

// commandOptions 保存只影响本次启动的命令行选项
type commandOptions struct {
	openUI    bool
	overrides dataConfigOverrides
}

// Run 执行单二进制命令入口
func Run(args []string) int {
	slog.SetDefault(slog.New(slog.NewJSONHandler(os.Stderr, nil)))
	err := runCommand(args)
	if errors.Is(err, flag.ErrHelp) {
		return 0
	}
	if err != nil {
		slog.Error("AIStudio2API 启动失败", "error", err)
		return 1
	}
	return 0
}

// runCommand 分派首次配置与默认服务
func runCommand(args []string) error {
	cfg, err := config.Load(".env")
	if err != nil {
		return err
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	if len(args) != 0 && args[0] == "setup" {
		return setup.Run(ctx, cfg, args[1:])
	}
	options, err := parseFlags(args, &cfg)
	if err != nil {
		return err
	}
	manager, err := newRuntimeManager(ctx, ".env", cfg, options.overrides)
	if err != nil {
		return err
	}
	return errors.Join(runServer(ctx, cfg, options, manager), manager.Close())
}

// parseFlags 使用命令行参数覆盖本次启动配置
func parseFlags(args []string, cfg *config.Config) (commandOptions, error) {
	flags := flag.NewFlagSet("aistudio2api", flag.ContinueOnError)
	flags.Usage = func() {
		fmt.Fprintln(flags.Output(), "首次配置: aistudio2api setup")
		fmt.Fprintln(flags.Output(), "日常启动: aistudio2api [参数]")
		flags.PrintDefaults()
	}
	authStates := flags.String("auth", cfg.AuthStates, "账户状态文件、目录或逗号分隔的多个路径")
	listenAddr := flags.String("listen", cfg.ListenAddr, "服务监听地址")
	proxy := flags.String("proxy", cfg.Proxy, "本次启动使用的 HTTP、HTTPS 或 SOCKS5 代理")
	openUI := flags.Bool("open-ui", len(args) == 0, "启动后打开管理界面")
	if err := flags.Parse(args); err != nil {
		return commandOptions{}, err
	}
	if flags.NArg() != 0 {
		return commandOptions{}, fmt.Errorf("未知参数 %q", flags.Arg(0))
	}

	cfg.AuthStates = strings.TrimSpace(*authStates)
	cfg.ListenAddr = strings.TrimSpace(*listenAddr)
	cfg.Proxy = strings.TrimSpace(*proxy)
	if err := cfg.Validate(); err != nil {
		return commandOptions{}, err
	}
	options := commandOptions{openUI: *openUI}
	flags.Visit(func(value *flag.Flag) {
		switch value.Name {
		case "auth":
			override := cfg.AuthStates
			options.overrides.authStates = &override
		case "proxy":
			override := cfg.Proxy
			options.overrides.proxy = &override
		}
	})
	return options, nil
}

// runServer 管理 HTTP 监听与优雅退出
func runServer(ctx context.Context, cfg config.Config, options commandOptions, manager *runtimeManager) error {
	manager.requests.log("service", "INFO", fmt.Sprintf("管理监听启动 | 地址=%s", cfg.ListenAddr))
	listener, err := net.Listen("tcp", cfg.ListenAddr)
	if err != nil {
		return fmt.Errorf("监听 %s: %w", cfg.ListenAddr, err)
	}
	apiHandler := api.NewHandler(manager, api.Config{APIKey: cfg.ProxyAPIKey, Admin: manager})
	server := &http.Server{
		Handler:           rootHandler(apiHandler, cfg.ProxyAPIKey),
		ReadHeaderTimeout: 10 * time.Second,
		IdleTimeout:       2 * time.Minute,
	}
	serveError := make(chan error, 1)
	go func() {
		serveError <- server.Serve(listener)
	}()

	address := browserAddress(listener.Addr().String())
	manager.requests.log("service", "INFO", "管理服务就绪 | 地址=http://"+address)
	if options.openUI {
		if err := openBrowser("http://" + address); err != nil {
			_ = server.Close()
			<-serveError
			return err
		}
		manager.requests.log("service", "INFO", "管理页面已打开 | 地址=http://"+address)
	}

	select {
	case err := <-serveError:
		if errors.Is(err, http.ErrServerClosed) {
			return nil
		}
		return err
	case <-ctx.Done():
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		if err := server.Shutdown(shutdownCtx); err != nil {
			return fmt.Errorf("关闭 HTTP 服务: %w", err)
		}
		if err := <-serveError; err != nil && !errors.Is(err, http.ErrServerClosed) {
			return err
		}
		return nil
	}
}

const loginPageHTML = `<!DOCTYPE html>
<html lang="zh-CN">
<head>
  <meta charset="UTF-8">
  <meta name="viewport" content="width=device-width, initial-scale=1.0">
  <title>AI Studio 控制台 - 登录</title>
  <style>
    * { box-sizing: border-box; margin: 0; padding: 0; }
    body {
      background-color: #0d1117;
      color: #e6edf3;
      font-family: -apple-system, BlinkMacSystemFont, "Segoe UI", Helvetica, Arial, sans-serif;
      display: flex;
      justify-content: center;
      align-items: center;
      min-height: 100vh;
      padding: 16px;
    }
    .card {
      background-color: #161b22;
      border: 1px solid #30363d;
      border-radius: 12px;
      padding: 32px;
      width: 100%;
      max-width: 400px;
      box-shadow: 0 8px 24px rgba(0,0,0,0.5);
    }
    .header { text-align: center; margin-bottom: 24px; }
    .icon { font-size: 36px; margin-bottom: 12px; }
    h1 { font-size: 20px; font-weight: 600; color: #f0f6fc; }
    p.sub { font-size: 13px; color: #8b949e; margin-top: 6px; }
    .form-group { margin-bottom: 20px; }
    label { display: block; font-size: 13px; font-weight: 500; margin-bottom: 8px; color: #c9d1d9; }
    input[type="password"] {
      width: 100%;
      padding: 10px 12px;
      background-color: #0d1117;
      border: 1px solid #30363d;
      border-radius: 6px;
      color: #c9d1d9;
      font-size: 14px;
      font-family: monospace;
      outline: none;
      transition: border-color 0.2s;
    }
    input:focus { border-color: #58a6ff; }
    button {
      width: 100%;
      padding: 10px;
      background-color: #238636;
      color: #ffffff;
      border: none;
      border-radius: 6px;
      font-size: 14px;
      font-weight: 600;
      cursor: pointer;
      transition: background-color 0.2s;
    }
    button:hover { background-color: #2ea043; }
    button:disabled { opacity: 0.6; cursor: not-allowed; }
    .error-msg { color: #f85149; font-size: 13px; margin-top: 12px; text-align: center; min-height: 18px; }
  </style>
</head>
<body>
  <div class="card">
    <div class="header">
      <div class="icon">✨</div>
      <h1>AI Studio 控制台</h1>
      <p class="sub">请输入 API Key 解锁管理端</p>
    </div>
    <form id="loginForm" onsubmit="handleLogin(event)">
      <div class="form-group">
        <label for="apiKey">API Key (密码)</label>
        <input type="password" id="apiKey" placeholder="sk-..." autofocus autocomplete="current-password" required>
      </div>
      <button type="submit" id="submitBtn">登 录</button>
      <div id="errMsg" class="error-msg"></div>
    </form>
  </div>
  <script>
    async function handleLogin(e) {
      e.preventDefault();
      const input = document.getElementById('apiKey');
      const btn = document.getElementById('submitBtn');
      const err = document.getElementById('errMsg');
      const key = input.value.trim();
      if (!key) return;

      btn.disabled = true;
      btn.innerText = '验证中...';
      err.innerText = '';

      try {
        const res = await fetch('/auth/login', {
          method: 'POST',
          headers: { 'Content-Type': 'application/json' },
          body: JSON.stringify({ key })
        });
        if (res.ok) {
          window.location.reload();
        } else {
          err.innerText = 'API Key 错误，请重试';
          btn.disabled = false;
          btn.innerText = '登 录';
          input.focus();
        }
      } catch (e) {
        err.innerText = '网络连接异常，请重试';
        btn.disabled = false;
        btn.innerText = '登 录';
      }
    }
  </script>
</body>
</html>`

// rootHandler 将公开 API 与内嵌管理端挂载到同一服务，并支持密码/API Key 鉴权保护
func rootHandler(apiHandler http.Handler, apiKey string) http.Handler {
	apiKey = strings.TrimSpace(apiKey)
	var sessionToken string
	if apiKey != "" {
		hash := sha256.Sum256([]byte(apiKey))
		sessionToken = hex.EncodeToString(hash[:])
	}

	uiHandler := webui.Handler()

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path := r.URL.Path

		// 1. 公开 API 路由直接放行 (内部自带 Bearer 鉴权)
		if strings.HasPrefix(path, "/v1/") || strings.HasPrefix(path, "/v1beta/") || path == "/health" {
			apiHandler.ServeHTTP(w, r)
			return
		}

		// 2. 未配置 API Key 时完全不拦截
		if apiKey == "" {
			if strings.HasPrefix(path, "/api/") {
				apiHandler.ServeHTTP(w, r)
			} else {
				uiHandler.ServeHTTP(w, r)
			}
			return
		}

		// 3. 处理登录请求
		if path == "/auth/login" {
			if r.Method == http.MethodPost {
				var req struct {
					Key string `json:"key"`
				}
				if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
					http.Error(w, `{"error":"invalid_json"}`, http.StatusBadRequest)
					return
				}
				if subtle.ConstantTimeCompare([]byte(strings.TrimSpace(req.Key)), []byte(apiKey)) == 1 {
					http.SetCookie(w, &http.Cookie{
						Name:     "omni_session",
						Value:    sessionToken,
						Path:     "/",
						MaxAge:   30 * 86400,
						HttpOnly: true,
						SameSite: http.SameSiteLaxMode,
					})
					w.Header().Set("Content-Type", "application/json")
					_, _ = w.Write([]byte(`{"ok":true}`))
					return
				}
				http.Error(w, `{"error":"invalid_key"}`, http.StatusUnauthorized)
				return
			}
			http.Error(w, "Method Not Allowed", http.StatusMethodNotAllowed)
			return
		}

		// 4. 处理退出登录
		if path == "/auth/logout" {
			http.SetCookie(w, &http.Cookie{
				Name:     "omni_session",
				Value:    "",
				Path:     "/",
				MaxAge:   -1,
				HttpOnly: true,
			})
			http.Redirect(w, r, "/", http.StatusFound)
			return
		}

		// 5. 检查授权
		isAuthed := false

		// 5.1 本机回环地址 (自启脚本、宿主机本地守护) 自动放行
		if host, _, err := net.SplitHostPort(r.RemoteAddr); err == nil {
			if ip := net.ParseIP(host); ip != nil && ip.IsLoopback() {
				isAuthed = true
			}
		}

		// 5.2 检查 Cookie
		if !isAuthed {
			if cookie, err := r.Cookie("omni_session"); err == nil && cookie.Value != "" {
				if subtle.ConstantTimeCompare([]byte(cookie.Value), []byte(sessionToken)) == 1 {
					isAuthed = true
				}
			}
		}

		// 5.3 检查 Authorization Header (如命令行调用 /api/)
		if !isAuthed {
			authHeader := strings.TrimSpace(r.Header.Get("Authorization"))
			if strings.HasPrefix(strings.ToLower(authHeader), "bearer ") {
				token := strings.TrimSpace(authHeader[7:])
				if subtle.ConstantTimeCompare([]byte(token), []byte(apiKey)) == 1 {
					isAuthed = true
				}
			}
		}

		// 已授权请求正常通过
		if isAuthed {
			if strings.HasPrefix(path, "/api/") {
				apiHandler.ServeHTTP(w, r)
			} else {
				uiHandler.ServeHTTP(w, r)
			}
			return
		}

		// 未授权时：如果是 /api/ 接口，返回 401 JSON
		if strings.HasPrefix(path, "/api/") {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusUnauthorized)
			_, _ = w.Write([]byte(`{"error":{"code":"unauthorized","message":"请先输入 API Key 登录管理端"}}`))
			return
		}

		// 允许加载静态资源
		if strings.HasPrefix(path, "/assets/") {
			uiHandler.ServeHTTP(w, r)
			return
		}

		// 未授权页面访问：渲染登录页面
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(loginPageHTML))
	})
}

// browserAddress 将通配监听地址转换为本机可访问地址
func browserAddress(address string) string {
	host, port, err := net.SplitHostPort(address)
	if err != nil {
		return address
	}
	if host == "" || host == "0.0.0.0" || host == "::" {
		host = "127.0.0.1"
	}
	return net.JoinHostPort(host, port)
}

// openBrowser 使用当前平台的系统命令打开管理界面
func openBrowser(url string) error {
	var command *exec.Cmd
	switch runtime.GOOS {
	case "windows":
		command = exec.Command("rundll32", "url.dll,FileProtocolHandler", url)
	case "darwin":
		command = exec.Command("open", url)
	default:
		command = exec.Command("xdg-open", url)
	}
	if err := command.Start(); err != nil {
		return fmt.Errorf("打开管理界面: %w", err)
	}
	if err := command.Process.Release(); err != nil {
		return fmt.Errorf("释放管理界面启动进程: %w", err)
	}
	return nil
}
