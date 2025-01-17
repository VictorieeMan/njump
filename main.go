package main

import (
	"context"
	"embed"
	"fmt"
	"html/template"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/kelseyhightower/envconfig"
	"github.com/rs/cors"
	"github.com/rs/zerolog"
)

type Settings struct {
	Port          string `envconfig:"PORT" default:"2999"`
	Domain        string `envconfig:"DOMAIN" default:"njump.me"`
	DiskCachePath string `envconfig:"DISK_CACHE_PATH" default:"/tmp/njump-cache"`
	TailwindDebug bool   `envconfig:"TAILWIND_DEBUG"`
}

//go:embed static/*
var static embed.FS

//go:embed templates/*
var templates embed.FS

var (
	s                  Settings
	log                = zerolog.New(os.Stderr).Output(zerolog.ConsoleWriter{Out: os.Stdout}).With().Timestamp().Logger()
	tailwindDebugStuff template.HTML
)

func updateArchives(ctx context.Context) {
	// do this so we don't run this every time we restart it locally
	time.Sleep(10 * time.Minute)

	for {
		select {
		case <-ctx.Done():
			return
		default:
			loadNpubsArchive(ctx)
			loadRelaysArchive(ctx)
		}
		time.Sleep(24 * time.Hour)
	}
}

func main() {
	err := envconfig.Process("", &s)
	if err != nil {
		log.Fatal().Err(err).Msg("couldn't process envconfig")
		return
	} else {
		if canonicalHost := os.Getenv("CANONICAL_HOST"); canonicalHost != "" {
			s.Domain = canonicalHost
		}
	}

	// if we're in tailwind debug mode, initialize the runtime tailwind stuff
	if s.TailwindDebug {
		configb, err := os.ReadFile("tailwind.config.js")
		if err != nil {
			log.Fatal().Err(err).Msg("failed to load tailwind.config.js")
			return
		}
		config := strings.Replace(string(configb), "module.exports", "tailwind.config", 1)

		styleb, err := os.ReadFile("tailwind.css")
		if err != nil {
			log.Fatal().Err(err).Msg("failed to load tailwind.css")
			return
		}
		style := string(styleb)

		tailwindDebugStuff = template.HTML(fmt.Sprintf("<script src=\"https://cdn.tailwindcss.com?plugins=typography\"></script><script>\n%s</script><style type=\"text/tailwindcss\">%s</style>", config, style))
	}

	// initialize disk cache
	defer cache.initialize()()

	// initialize the function to update the npubs/relays archive
	ctx := context.Background()
	go updateArchives(ctx)

	// routes
	mux := http.NewServeMux()
	mux.Handle("/njump/static/", http.StripPrefix("/njump/", http.FileServer(http.FS(static))))
	mux.HandleFunc("/relays-archive.xml", renderArchive)
	mux.HandleFunc("/npubs-archive.xml", renderArchive)
	mux.HandleFunc("/services/oembed", renderOEmbed)
	mux.HandleFunc("/relays-archive/", renderArchive)
	mux.HandleFunc("/npubs-archive/", renderArchive)
	mux.HandleFunc("/njump/image/", renderImage)
	mux.HandleFunc("/njump/proxy/", proxy)
	mux.HandleFunc("/robots.txt", renderRobots)
	mux.HandleFunc("/r/", renderRelayPage)
	mux.HandleFunc("/random", redirectToRandom)
	mux.HandleFunc("/e/", redirectFromESlash)
	mux.HandleFunc("/p/", redirectFromPSlash)
	mux.HandleFunc("/favicon.ico", redirectToFavicon)
	mux.HandleFunc("/", renderEvent)

	log.Print("listening at http://0.0.0.0:" + s.Port)
	if err := http.ListenAndServe("0.0.0.0:"+s.Port, cors.Default().Handler(mux)); err != nil {
		log.Fatal().Err(err).Msg("")
	}

	select {}
}
