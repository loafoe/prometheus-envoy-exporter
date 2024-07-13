# prometheus-envoy-exporter

Prometheus exporter for near Realtime Enphase Envoy-S Metered local readouts

## Features

- Support for new D7 firmware
- Near real-time refresh of data (default every 20 seconds)
- Discovery via mDNS
- Individual micro-inverter data
- Cloud credentials or JWT token based authentication

## Disclaimer

This exporter is not endorsed or approved by Enphase. Use at your own risk. 

## Compatibility

The exporter is compatible with newer `D7.x` firmware versions only which uses long lived JWT and short lived sessionId based authentication. Support for older D5.x firmware is not available but should be trivial to add if there is a need.

## Installation

Using Go 1.19 or newer

```shell
go install github.com/loafoe/prometheus-envoy-exporter@latest
```

## Usage

### Configure environment

| Environment      | Description                              | Required | Default               |
|------------------|------------------------------------------|----------|-----------------------|
| `ENVOY_USERNAME` | Username for Enlighten cloud             | N        |                       |
| `ENVOY_PASSWORD` | Password for Enlighten cloud             | N        |                       |
| `ENVOY_SERIAL`   | Serial number of the gateway on your LAN | N        |                       |
| `ENVOY_LISTEN`   | Listen port of the exporter              | N        | `0.0.0.0:8899`        |
| `ENVOY_ADDRESS`  | Address of the Envoy-S gateway           | N        | `https://envoy.local` |
| `ENVOY_JWT`      | Long lived JWT token                     | N        |                       |
| `ENVOY_REFRESH`  | Seconds to wait between refreshing data  | N        | `20`                  |

> When you set only a JWT be sure to refresh it at least once a year, otherwise set your Enlighten Cloud login credentials

> When not setting a serial the exporter will attempt to use `mDNS` to discover the Gateway on your local LAN

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

### Ship to prometheus / mimir

You can use something like Grafana Alloy to ship data to a remote write endpoint. Example:

```yml
prometheus.scrape "metrics_default_envoy_exporter" {
	targets = [{
		__address__ = "localhost:8899",
	}]
	forward_to      = [prometheus.remote_write.metrics_default.receiver]
	job_name        = "envoy_exporter"
	scrape_interval = "30s"
}

prometheus.remote_write "metrics_default" {
	external_labels = {
		environment = "home",
	}

	endpoint {
		name = "mimir"
		url  = "https://some.cloud.com/api/prom/push"

		basic_auth {
			username = "some_login"
			password = "some_password"
		}

	}
}
```

## License

License is MIT
