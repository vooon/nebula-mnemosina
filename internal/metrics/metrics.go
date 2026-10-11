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

	retentionRunsTotal = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "nebula_mnemosina_retention_runs_total",
		Help: "Total retention runs.",
	}, []string{"success"})

	retentionDeletedRowsTotal = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "nebula_mnemosina_retention_deleted_rows_total",
		Help: "Total rows deleted by retention. Rows removed by cascade are not counted.",
	}, []string{"table"})

	retentionDroppedChunksTotal = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "nebula_mnemosina_retention_dropped_chunks_total",
		Help: "Total TimescaleDB chunks dropped by retention.",
	}, []string{"table"})
)

func ObservePoll(result model.PollResult) {
	pollsTotal.WithLabelValues(result.Lighthouse.Name, strconv.FormatBool(result.Success())).Inc()
	pollDuration.WithLabelValues(result.Lighthouse.Name).Observe(result.Duration().Seconds())
	for _, command := range result.Commands {
		commandTotal.WithLabelValues(result.Lighthouse.Name, command.Command, strconv.FormatBool(command.Success())).Inc()
		commandDuration.WithLabelValues(result.Lighthouse.Name, command.Command).Observe(command.Duration().Seconds())
	}
}

func ObserveRetentionRun(success bool) {
	retentionRunsTotal.WithLabelValues(strconv.FormatBool(success)).Inc()
}

func ObserveRetentionDeletedRows(table string, rows int64) {
	retentionDeletedRowsTotal.WithLabelValues(table).Add(float64(rows))
}

func ObserveRetentionDroppedChunks(table string, chunks int64) {
	retentionDroppedChunksTotal.WithLabelValues(table).Add(float64(chunks))
}
