# prometheus-envoy-exporter

Prometheus exporter for (near) Realtime Enphase Envoy-S Metered readouts

## Disclaimer

This exporter is not endorsed or approved by Enphase.

## Install

Using Go 1.19 or newer

```shell
go install github.com/loafoe/prometheus-envoy-exporter@latest
```

## Usage

### Configure environment

| Environment | Description | Required | Default |
|-------------|-------------|----------|---------|
| `ENVOY_USERNAME` | Username for Enlighten cloud | Y | |
| `ENVOY_PASSWORD` | Password fro Enlighten cloud | Y | |
| `ENVOY_LISTEN` | Listen port of the exporter | N | `0.0.0.0:8899` |
| `ENVOY_ADDRESS` | Address of the Envoy-S gateway | N | `https://envoy.local` |

### Run exporter

```shell
prometheus-envoy-exporter
```

### Output

```
# HELP envoy_inverter_last_report_watts Generated watts by inverter
# TYPE envoy_inverter_last_report_watts gauge
envoy_inverter_last_report_watts{gateway="122220000001",serial="48224000001"} 10
envoy_inverter_last_report_watts{gateway="122220000001",serial="48224000002"} 9
envoy_inverter_last_report_watts{gateway="122220000001",serial="48224000003"} 8
envoy_inverter_last_report_watts{gateway="122220000001",serial="48224000004"} 8
envoy_inverter_last_report_watts{gateway="122220000001",serial="48224000005"} 9
envoy_inverter_last_report_watts{gateway="122220000001",serial="48224000006"} 8
envoy_inverter_last_report_watts{gateway="122220000001",serial="48224000007"} 9
envoy_inverter_last_report_watts{gateway="122220000001",serial="48224000008"} 9
envoy_inverter_last_report_watts{gateway="122220000001",serial="48224000009"} 8
envoy_inverter_last_report_watts{gateway="122220000001",serial="48224000010"} 8
envoy_inverter_last_report_watts{gateway="122220000001",serial="48224000011"} 8
# HELP envoy_jwt_refreshes Number of JWT token refreshes during runtime
# TYPE envoy_jwt_refreshes counter
envoy_jwt_refreshes{gateway="122220000001"} 1
# HELP envoy_production_watts_now Watts being produced now
# TYPE envoy_production_watts_now gauge
envoy_production_watts_now{gateway="122220000001"} 135
# HELP envoy_production_wh_lifetime Watt-hour generated over lifetime
# TYPE envoy_production_wh_lifetime gauge
envoy_production_wh_lifetime{gateway="122220000001"} 64783
# HELP envoy_session_refreshes Number of session refreshes during runtime
# TYPE envoy_session_refreshes gauge
envoy_session_refreshes{gateway="122220000001"} 1
# HELP envoy_session_uses Number of session reuses during runtime
# TYPE envoy_session_uses gauge
envoy_session_uses{gateway="122220000001"} 102
```

### Ship to prometheus

You can use something like Grafana-agent to ship data to a remote write endpoint. Example:

```yml
metrics:
  configs:
    - name: default
      scrape_configs:
        - job_name: 'envoy_exporter'
          scrape_interval: 30s
          static_configs:
            - targets: ['localhost:8899']
      remote_write:
        - url: https://prometheus.example.com/api/v1/write
          basic_auth:
            username: scraper
            password: S0m3pAssW0rdH3Re
```

## License

License is MIT
