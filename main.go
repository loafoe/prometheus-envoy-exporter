package main

import (
	"fmt"
	"net/http"
	"os"
	"time"

	"github.com/spf13/viper"

	"github.com/loafoe/go-envoy"

	"golang.org/x/exp/slog"

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
	viper.AutomaticEnv()
	viper.SetEnvPrefix("envoy")
	viper.SetDefault("address", "https://envoy.local")
	viper.SetDefault("listen", "0.0.0.0:8899")
	viper.SetDefault("debug", false)
	viper.SetDefault("refresh", 20)

	username := viper.GetString("username")
	password := viper.GetString("password")
	serial := viper.GetString("serial")
	address := viper.GetString("address")
	listenAddr := viper.GetString("listen")
	jwt := viper.GetString("jwt")
	debug := viper.GetBool("debug")
	refresh := viper.GetInt("refresh")

	slog.SetDefault(slog.New(slog.NewJSONHandler(os.Stdout)))

	if serial == "" { // Discovery via mDNS
		discover, err := envoy.Discover()
		if err != nil {
			fmt.Printf("Missing serial and failed discovery: %s\n", err)
			os.Exit(1)
		}
		fmt.Printf("Using discovered envoy at %s with serial %s\n", discover.IPV4, discover.Serial)
		serial = discover.Serial
		address = fmt.Sprintf("https://%s", discover.IPV4)
	}

	e, err := envoy.NewClient(username, password, serial,
		envoy.WithGatewayAddress(address),
		envoy.WithDebug(debug),
		envoy.WithJWT(jwt),
		envoy.WithNotification(&notification{serial: serial}))

	if err != nil {
		fmt.Printf("Quitting because of error opening envoy: %v\n", err)
		os.Exit(1)
	}

	go func() {
		for {
			cr, resp, err := e.CommCheck()
			if err != nil {
				if resp != nil && resp.StatusCode == http.StatusUnauthorized {
					e.InvalidateSession() // Token expired?
				}
			}
			if cr != nil {
				slog.Info("Found devices", "count", len(*cr))
			}

			prod, _, err := e.Production()
			if err != nil {
				slog.Error("error", err)
			}
			if prod != nil && len(prod.Production) > 0 {
				productionWattsNow.WithLabelValues(serial).Set(prod.Production[0].WNow)
				productionWattHourLifetime.WithLabelValues(serial).Set(prod.Production[0].WhLifetime)
			}

			inverters, _, err := e.Inverters()
			if err != nil {
				slog.Error("error", err)
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

	_ = http.ListenAndServe(listenAddr, nil)
}

type notification struct {
	lastSession string
	serial      string
	uses        float64
}

func (n *notification) JWTRefreshed(_ string) {
	slog.Info("JWT refreshed")
	jwtRefreshes.WithLabelValues(n.serial).Inc()
}

func (n *notification) JWTError(err error) {
	slog.Error("JWT error", err)
}

func (n *notification) SessionRefreshed(s string) {
	slog.Info("Session refreshed", "session", s)
	sessionRefreshes.WithLabelValues(n.serial).Inc()
}

func (n *notification) SessionUsed(s string) {
	if n.lastSession != s {
		n.lastSession = s
		n.uses = 0
	}
	n.uses = n.uses + 1
	sessionUses.WithLabelValues(n.serial).Set(n.uses)
	slog.Info("Session used", "session", s)
}

func (n *notification) SessionError(err error) {
	slog.Error("session error", err)
}
