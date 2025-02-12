package main

import (
	"errors"
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
	viper.SetEnvPrefix("envoy")
	viper.SetConfigName("envoy")
	viper.SetConfigType("yaml")
	viper.AddConfigPath("/etc/envoy/")
	viper.SetDefault("address", "https://envoy.local")
	viper.SetDefault("listen", "0.0.0.0:8899")
	viper.SetDefault("debug", false)
	viper.SetDefault("refresh", 20)

	if err := viper.ReadInConfig(); err != nil {
		var configFileNotFoundError viper.ConfigFileNotFoundError
		if errors.As(err, &configFileNotFoundError) {
			slog.Info("Config file not found")
		}
	}

	// Logger
	programLevel := new(slog.LevelVar)
	h := slog.NewJSONHandler(os.Stderr, &slog.HandlerOptions{Level: programLevel})
	slog.SetDefault(slog.New(h))

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

	if serial == "" { // Discovery via mDNS
		discover, err := envoy.Discover()
		if err != nil {
			slog.Error("Missing serial and failed discovery", "error", err)
			os.Exit(2)
		}
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
