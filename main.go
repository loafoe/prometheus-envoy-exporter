package main

import (
	"errors"
	"flag"
	"fmt"
	"net/http"
	"os"
	"time"

	"github.com/spf13/viper"

	"github.com/loafoe/go-envoy"

	"log/slog"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

var metricNamePrefix = "envoy_"

var (
	registry           = prometheus.NewRegistry()
	productionWattsNow = prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Name: metricNamePrefix + "production_watts_now",
		Help: "Watts being produced now",
	}, []string{"gateway"})
	productionWattHourLifetime = prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Name: metricNamePrefix + "production_wh_lifetime",
		Help: "Watt-hour generated over lifetime",
	}, []string{"gateway"})
	sessionRefreshes = prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Name: metricNamePrefix + "session_refreshes",
		Help: "Number of session refreshes during runtime",
	}, []string{"gateway"})
	sessionUses = prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Name: metricNamePrefix + "session_uses",
		Help: "Number of session reuses during runtime",
	}, []string{"gateway"})
	jwtRefreshes = prometheus.NewCounterVec(prometheus.CounterOpts{
		Name: metricNamePrefix + "jwt_refreshes",
		Help: "Number of JWT token refreshes during runtime",
	}, []string{"gateway"})
	inverterLastReportWatts = prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Name: metricNamePrefix + "inverter_last_report_watts",
		Help: "Generated watts by inverter",
	}, []string{"gateway", "serial"})
)

func init() {
	registry.MustRegister(productionWattsNow)
	registry.MustRegister(productionWattHourLifetime)
	registry.MustRegister(jwtRefreshes)
	registry.MustRegister(sessionRefreshes)
	registry.MustRegister(sessionUses)
	registry.MustRegister(inverterLastReportWatts)
}

func main() {
	var configPath string
	flag.StringVar(&configPath, "config", "", "Path to configuration file")
	flag.Usage = func() {
		fmt.Fprintf(os.Stderr, "Usage of %s:\n", os.Args[0])
		fmt.Fprintf(os.Stderr, "  %s [options]\n\n", os.Args[0])
		fmt.Fprintf(os.Stderr, "Options:\n")
		flag.PrintDefaults()
		fmt.Fprintf(os.Stderr, "\nEnvironment variables:\n")
		fmt.Fprintf(os.Stderr, "  ENVOY_USERNAME   Username for Enlighten cloud\n")
		fmt.Fprintf(os.Stderr, "  ENVOY_PASSWORD   Password for Enlighten cloud\n")
		fmt.Fprintf(os.Stderr, "  ENVOY_SERIAL     Serial number of the gateway on your LAN\n")
		fmt.Fprintf(os.Stderr, "  ENVOY_LISTEN     Listen port of the exporter (default \"0.0.0.0:8899\")\n")
		fmt.Fprintf(os.Stderr, "  ENVOY_ADDRESS    Address of the Envoy-S gateway (default \"https://envoy.local\")\n")
		fmt.Fprintf(os.Stderr, "  ENVOY_JWT        Long lived JWT token\n")
		fmt.Fprintf(os.Stderr, "  ENVOY_REFRESH    Seconds to wait between refreshing data (default 20)\n")
		fmt.Fprintf(os.Stderr, "  ENVOY_DEBUG      Enable debug logging (default false)\n")
	}
	flag.Parse()

	viper.SetEnvPrefix("envoy")
	viper.AutomaticEnv()

	viper.SetDefault("address", "https://envoy.local")
	viper.SetDefault("listen", "0.0.0.0:8899")
	viper.SetDefault("debug", false)
	viper.SetDefault("refresh", 20)
	viper.AutomaticEnv()

	if configPath != "" {
		viper.SetConfigFile(configPath)
	} else {
		viper.SetConfigName("envoy")
		viper.SetConfigType("yaml")
		viper.AddConfigPath("/etc/envoy/")
		viper.AddConfigPath(".")
	}

	// Logger setup early so we can log config errors using slog
	programLevel := new(slog.LevelVar)
	h := slog.NewJSONHandler(os.Stderr, &slog.HandlerOptions{Level: programLevel})
	slog.SetDefault(slog.New(h))

	if err := viper.ReadInConfig(); err != nil {
		var configFileNotFoundError viper.ConfigFileNotFoundError
		if errors.As(err, &configFileNotFoundError) {
			if configPath != "" {
				slog.Error("Specified config file not found", "path", configPath, "error", err)
				os.Exit(1)
			}
			slog.Debug("Config file not found", "error", err)
		} else {
			slog.Error("Error reading config file", "error", err)
			os.Exit(1)
		}
	} else {
		slog.Info("Using config file", "path", viper.ConfigFileUsed())
	}

	if viper.GetBool("debug") {
		programLevel.Set(slog.LevelDebug)
	}
	slog.Debug("Settings", "AllSettings", viper.AllSettings())

	username := viper.GetString("username")
	password := viper.GetString("password")
	serial := viper.GetString("serial")
	address := viper.GetString("address")
	listenAddr := viper.GetString("listen")
	jwt := viper.GetString("jwt")
	debug := viper.GetBool("debug")
	refresh := viper.GetInt("refresh")

	// Validate credentials early
	if jwt == "" && (username == "" || password == "") {
		slog.Error("Quitting because credentials are incomplete. Please set either ENVOY_JWT or both ENVOY_USERNAME and ENVOY_PASSWORD (via environment variables, command-line arguments, or config file)")
		os.Exit(3)
	}

	// Discovery via mDNS - overrides config when successful
	discover, err := envoy.Discover()
	if err != nil {
		if serial == "" {
			slog.Error("No serial configured and discovery failed", "error", err)
			os.Exit(2)
		}
		slog.Warn("Discovery failed, using configured values", "error", err)
	} else {
		slog.Info("Using discovered envoy", "envoy_ip", discover.IPV4, "serial", discover.Serial)
		serial = discover.Serial
		if discover.IPV4 != "<nil>" {
			address = fmt.Sprintf("https://%s", discover.IPV4)
		}
	}

	e, err := envoy.NewClient(username, password, serial,
		envoy.WithGatewayAddress(address),
		envoy.WithDebug(debug),
		envoy.WithJWT(jwt),
		envoy.WithNotification(&notification{serial: serial, logger: slog.Default()}))

	if err != nil {
		slog.Error("Quitting because of error opening envoy", "error", err)
		os.Exit(3)
	}

	go func() {
		for {
			cr, err := e.CommCheck()
			if err != nil {
				e.InvalidateSession() // Token expired?
			}
			if cr != nil {
				slog.Info("Found devices", "count", len(*cr))
			}

			prod, err := e.Production()
			if err != nil {
				slog.Error("error getting production data", "error", err)
			}
			if prod != nil && len(prod.Production) > 0 {
				productionWattsNow.WithLabelValues(serial).Set(prod.Production[0].WNow)
				productionWattHourLifetime.WithLabelValues(serial).Set(prod.Production[0].WhLifetime)
			}

			inverters, err := e.Inverters()
			if err != nil {
				slog.Error("error getting inverters data", "error", err)
			}
			if inverters != nil {
				for _, inverter := range *inverters {
					inverterLastReportWatts.WithLabelValues(serial, inverter.SerialNumber).Set(float64(inverter.LastReportWatts))
				}
			}
			time.Sleep(time.Duration(refresh) * time.Second)
		}
	}()

	slog.Info("Start listening", slog.String("address", listenAddr))
	http.Handle("/metrics", promhttp.HandlerFor(registry, promhttp.HandlerOpts{}))

	err = http.ListenAndServe(listenAddr, nil)
	slog.Info("Program exit", "result", err)
}

type notification struct {
	logger      *slog.Logger
	lastSession string
	serial      string
	uses        float64
}

func (n *notification) JWTRefreshed(_ string) {
	n.logger.Debug("JWT refreshed")
	jwtRefreshes.WithLabelValues(n.serial).Inc()
}

func (n *notification) JWTError(err error) {
	n.logger.Error("JWT error", "error", err)
}

func (n *notification) SessionRefreshed(s string) {
	n.logger.Debug("Session refreshed", "session", s)
	sessionRefreshes.WithLabelValues(n.serial).Inc()
}

func (n *notification) SessionUsed(s string) {
	if n.lastSession != s {
		n.lastSession = s
		n.uses = 0
	}
	n.uses = n.uses + 1
	sessionUses.WithLabelValues(n.serial).Set(n.uses)
	n.logger.Debug("Session used", "session", s)
}

func (n *notification) SessionError(err error) {
	n.logger.Error("session error", "error", err)
}
