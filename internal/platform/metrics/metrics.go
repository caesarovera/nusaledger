// Package metrics mendaftarkan metrik Prometheus minimum Fase 1 (docs/03 §8).
package metrics

import (
	"net/http"
	"strconv"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/collectors"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

type Metrics struct {
	registry *prometheus.Registry

	HTTPDuration  *prometheus.HistogramVec
	HTTPRequests  *prometheus.CounterVec
	DBConnsInUse  prometheus.Gauge
	DBConnsMax    prometheus.Gauge
	Transactions  *prometheus.CounterVec
	BalanceDrift  prometheus.Gauge // HARUS selalu 0
	TrialBalance  prometheus.Gauge // HARUS selalu 0
	LoginFailures *prometheus.CounterVec
}

func New() *Metrics {
	m := &Metrics{
		registry: prometheus.NewRegistry(),
		HTTPDuration: prometheus.NewHistogramVec(prometheus.HistogramOpts{
			Name: "http_request_duration_seconds", Help: "Durasi request HTTP.",
			Buckets: []float64{.005, .01, .025, .05, .1, .2, .3, .5, 1, 2.5, 5},
		}, []string{"method", "route", "status"}),
		HTTPRequests: prometheus.NewCounterVec(prometheus.CounterOpts{
			Name: "http_requests_total", Help: "Jumlah request HTTP.",
		}, []string{"method", "route", "status"}),
		DBConnsInUse: prometheus.NewGauge(prometheus.GaugeOpts{Name: "db_pool_conns_in_use", Help: "Koneksi pool yang sedang dipakai."}),
		DBConnsMax:   prometheus.NewGauge(prometheus.GaugeOpts{Name: "db_pool_conns_max", Help: "Batas koneksi pool."}),
		Transactions: prometheus.NewCounterVec(prometheus.CounterOpts{
			Name: "ledger_transactions_total", Help: "Transaksi ledger per jenis dan hasil.",
		}, []string{"type", "status"}),
		BalanceDrift:  prometheus.NewGauge(prometheus.GaugeOpts{Name: "ledger_balance_drift_total", Help: "Jumlah akun yang saldonya tidak cocok dengan entries. HARUS 0."}),
		TrialBalance:  prometheus.NewGauge(prometheus.GaugeOpts{Name: "ledger_trial_balance_difference", Help: "Σ debit − Σ kredit seluruh ledger. HARUS 0."}),
		LoginFailures: prometheus.NewCounterVec(prometheus.CounterOpts{Name: "auth_login_failures_total", Help: "Login gagal per alasan."}, []string{"reason"}),
	}
	m.registry.MustRegister(
		collectors.NewGoCollector(), collectors.NewProcessCollector(collectors.ProcessCollectorOpts{}),
		m.HTTPDuration, m.HTTPRequests, m.DBConnsInUse, m.DBConnsMax, m.Transactions, m.BalanceDrift, m.TrialBalance, m.LoginFailures,
	)
	return m
}

// Handler menyajikan /metrics.
func (m *Metrics) Handler() http.Handler {
	return promhttp.HandlerFor(m.registry, promhttp.HandlerOpts{})
}

// ObserveHTTP mencatat satu request. route memakai pola (/transactions/{id}), bukan path mentah.
func (m *Metrics) ObserveHTTP(method, route string, status int, d time.Duration) {
	s := strconv.Itoa(status)
	m.HTTPDuration.WithLabelValues(method, route, s).Observe(d.Seconds())
	m.HTTPRequests.WithLabelValues(method, route, s).Inc()
}
