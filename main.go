package main

import (
	"fmt"
	"net/http"
	"os"
	"strconv"
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
	productionWattsNow = prometheus.NewGauge(prometheus.GaugeOpts{
		Name: metricNamePrefix + "production_watts_now",
		Help: "Watts being produced now",
	})
	productionWattHourLifetime = prometheus.NewGauge(prometheus.GaugeOpts{
		Name: metricNamePrefix + "production_wh_lifetime",
		Help: "Watt-hour generated over lifetime",
	})
)

func init() {
	registry.MustRegister(productionWattsNow)
	registry.MustRegister(productionWattHourLifetime)
}

func floatValue(input string) (fval float64) {
	fval, _ = strconv.ParseFloat(input, 64)
	return
}

func main() {
	viper.AutomaticEnv()
	viper.SetEnvPrefix("envoy")
	viper.SetDefault("address", "https://envoy.local")
	viper.SetDefault("listen", "0.0.0.0:8899")

	username := viper.GetString("username")
	password := viper.GetString("password")
	serial := viper.GetString("serial")
	address := viper.GetString("address")
	listenAddr := viper.GetString("listen")

	slog.SetDefault(slog.New(slog.NewTextHandler(os.Stderr)))

	e, err := envoy.NewClient(envoy.WithEnlightenBase(address),
		envoy.WithSerial(serial),
		envoy.WithCredentials(username, password))

	if err != nil {
		fmt.Printf("Quitting because of error opening envoy: %v", err)
		os.Exit(1)
	}

	// Start
	//e.Start()

	go func() {
		resp, err := e.Production()
		if err != nil {
			slog.Error("error", err)
		}
		if resp != nil && len(resp.Production) > 0 {
			productionWattsNow.Set(resp.Production[0].WNow)
		}
		time.Sleep(5 * time.Second)
	}()

	slog.Info("Start listening", slog.String("address", listenAddr))
	http.Handle("/metrics", promhttp.HandlerFor(registry, promhttp.HandlerOpts{}))

	_ = http.ListenAndServe(listenAddr, nil)
}
