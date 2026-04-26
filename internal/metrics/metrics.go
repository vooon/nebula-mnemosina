package metrics

import (
	"strconv"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"

	"github.com/vooon/nebula-mnemosina/internal/model"
)

var (
	pollsTotal = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "nebula_mnemosina_polls_total",
		Help: "Total Nebula lighthouse poll attempts.",
	}, []string{"lighthouse", "success"})

	pollDuration = promauto.NewHistogramVec(prometheus.HistogramOpts{
		Name:    "nebula_mnemosina_poll_duration_seconds",
		Help:    "Nebula lighthouse poll duration.",
		Buckets: prometheus.DefBuckets,
	}, []string{"lighthouse"})

	commandTotal = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "nebula_mnemosina_ssh_commands_total",
		Help: "Total Nebula SSH commands.",
	}, []string{"lighthouse", "command", "success"})

	commandDuration = promauto.NewHistogramVec(prometheus.HistogramOpts{
		Name:    "nebula_mnemosina_ssh_command_duration_seconds",
		Help:    "Nebula SSH command duration.",
		Buckets: prometheus.DefBuckets,
	}, []string{"lighthouse", "command"})
)

func ObservePoll(result model.PollResult) {
	pollsTotal.WithLabelValues(result.Lighthouse.Name, strconv.FormatBool(result.Success())).Inc()
	pollDuration.WithLabelValues(result.Lighthouse.Name).Observe(result.Duration().Seconds())
	for _, command := range result.Commands {
		commandTotal.WithLabelValues(result.Lighthouse.Name, command.Command, strconv.FormatBool(command.Success())).Inc()
		commandDuration.WithLabelValues(result.Lighthouse.Name, command.Command).Observe(command.Duration().Seconds())
	}
}
