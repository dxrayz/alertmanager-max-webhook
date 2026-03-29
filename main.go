package main

import (
	"bytes"
	"context"
	"encoding/json"
	"flag"
	"html/template"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"path"
	"strconv"
	"strings"
	"syscall"
	"time"

	maxbot "github.com/max-messenger/max-bot-api-client-go"
	"github.com/max-messenger/max-bot-api-client-go/schemes"
	alertmanager_template "github.com/prometheus/alertmanager/template"
)

var (
    listen_address = flag.String("listen-address", ":9096", "The address to listen on for HTTP requests.")
	templates_path = flag.String("templates-path", "/templates/*.tmpl", "The templates file path in glob format")
	api_client_timeout = flag.String("api-client-timeout", "5", "MAX API client timeout in seconds")
	templates = template.New(path.Base(*templates_path))
	max_bot_token = os.Getenv("MAX_BOT_TOKEN")
	logger = slog.New(slog.NewTextHandler(os.Stdout, nil))
)

const max_message_length int = 4000
const default_template = `
{{/* MAX default template BEGIN*/}}
{{- define "__text_alerts_list" }}{{- range . }}
❗ {{ .Annotations.summary }}
{{- if eq .Labels.severity "critical" }}
🚨 CRITICAL: {{ .Annotations.description }}{{- end }}
{{- if eq .Labels.severity "warning" }}
⚠️ WARNING: {{ .Annotations.description }}{{- end }}
🏷️ Labels:
{{- range .Labels.SortedPairs }}
- {{ .Name }}: {{ .Value }}
{{- end }}{{ printf "\n" }}{{- end }}{{- end }}

{{- define "max.default.message" }}
{{- if gt (len .Alerts.Firing) 0 }}
🔥FIRING🔥: {{ template "__text_alerts_list" .Alerts.Firing }}
{{ end }}
{{- if gt (len .Alerts.Resolved) 0 }}
✅RESOLVED✅: {{ template "__text_alerts_list" .Alerts.Resolved }}
{{ end }}🛠 <a href="{{ .ExternalURL }}">Alertmanager</a> 🛠
{{- end }}
{{/* MAX default template END*/}}
`

func Log(handler http.Handler) http.Handler {
    return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/health" {
			handler.ServeHTTP(w, r)
			return
		}
		logger.Info("access log",
			"remote", r.RemoteAddr,
			"method", r.Method,
			"path", r.URL)
	handler.ServeHTTP(w, r)
    })
}

func parse_templates() (string) {
	tmpl, err := template.New(path.Base(*templates_path)).ParseGlob(*templates_path)
	if err != nil && strings.Contains(err.Error(), "pattern matches no files") {
		logger.Warn(err.Error())
		return err.Error()
	} else if err != nil {
		logger.Error(err.Error())
		return err.Error()
	}
	templates = tmpl
	return ""
}

func main() {
    flag.Parse()

	api_client_timeout, err := strconv.ParseInt(*api_client_timeout, 10, 8)
	if err != nil {
		logger.Error("api-client-timeout must be in int type")
		return
	}
    max_api_opts := []maxbot.Option{
        maxbot.WithHTTPClient(&http.Client{Timeout: time.Duration(api_client_timeout) * time.Second}),
        maxbot.WithPauseTimeout(30 * time.Second),
    }
	parse_templates()
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGTERM, os.Interrupt)
	defer stop()

	default_tmpl, err := template.New("default").Parse(default_template)
	if err != nil {
		logger.Error(err.Error())
	}

    sigs := make(chan os.Signal, 1)
	signal.Notify(sigs, syscall.SIGHUP, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		for sig := range sigs {
			switch sig {
			case syscall.SIGHUP:
				logger.Info("Received SIGHUP, reloading configuration...")
				err := parse_templates();
				if err != "" {
					logger.Error("Error reloading config:", "msg", err)
				} else {
					logger.Info("Configuration reloaded successfully.")
				}
			case syscall.SIGINT, syscall.SIGTERM:
				logger.Info("Received termination signal. Shutting down gracefully...\n", "msg", sig)
				os.Exit(0)
			}
		}
	}()

	http.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) { http.Error(w, "OK", http.StatusOK) })
    http.HandleFunc("/alert/{chat_id}", func(w http.ResponseWriter, r *http.Request) {
		defer r.Body.Close()
		var data alertmanager_template.Data
		err := json.NewDecoder(r.Body).Decode(&data)
		if err != nil {
			log_message := err.Error()
			logger.Error(log_message)
			http.Error(w, log_message, http.StatusBadRequest)
			return
		}
		template_name := r.URL.Query().Get("template-name")
		message_format := r.URL.Query().Get("message-format")
		if message_format == "" {
			message_format = "html"
		}

		chat_id_r := r.PathValue("chat_id")
		chat_id, err := strconv.ParseInt(chat_id_r, 10, 64)
		if err != nil {
			log_message := "Request error: {chat_id} must be in int64 type"
			logger.Error(log_message)
			http.Error(w, log_message, http.StatusInternalServerError)
			return
		} else {
			api, err := maxbot.New(max_bot_token, max_api_opts...)
			if err != nil {
				log_message := err.Error()
				logger.Error(log_message)
				http.Error(w, log_message, http.StatusInternalServerError)
				return
			}

			buf := new(bytes.Buffer)
			if template_name == "" {
				err = default_tmpl.ExecuteTemplate(buf, "max.default.message", data)
			} else {
				err = templates.ExecuteTemplate(buf, template_name, data)
			}
			if err != nil {
				log_message := err.Error()
				logger.Error(log_message)
				http.Error(w, log_message, http.StatusInternalServerError)
				return
			}

			if buf.Len() >= max_message_length {
				for i := 0; i < buf.Len(); i += max_message_length {
					end := i + max_message_length
					end = min(end, buf.Len())
					message := maxbot.NewMessage().SetChat(chat_id).SetText(buf.String()[i:end]).SetFormat(schemes.Format(message_format))
					err = api.Messages.Send(ctx, message)
				}
			} else {
				message := maxbot.NewMessage().SetChat(chat_id).SetText(buf.String()).SetFormat(schemes.Format(message_format))
				err = api.Messages.Send(ctx, message)
			}
			if err != nil {
				log_message := err.Error()
				logger.Error(log_message)
				http.Error(w, log_message, http.StatusInternalServerError)
				return
			}
		}
    })

    logger.Info("Starting server at port", "address", *listen_address)
	err = http.ListenAndServe(*listen_address, Log(http.DefaultServeMux))
    if err != nil {
        logger.Info("Error starting the server:", "msg", err)
    }
}
