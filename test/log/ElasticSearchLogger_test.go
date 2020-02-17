package test_log

import (
	"os"
	"testing"

	cconf "github.com/pip-services3-go/pip-services3-commons-go/config"
	elog "github.com/pip-services3-node/pip-services3-elasticsearch-go/log"
	"github.com/stretchr/testify/assert"
)

func TestElasticSearchLogger(t *testing.T) {
	var logger *elog.ElasticSearchLogger
	var fixture *LoggerFixture

	var host = os.Getenv("ELASTICSEARCH_SERVICE_HOST")
	if host == "" {
		host = "localhost"
	}

	var port = os.Getenv("ELASTICSEARCH_SERVICE_PORT")
	if port == "" {
		port = "9200"
	}

	logger = elog.NewElasticSearchLogger()
	fixture = NewLoggerFixture(logger.CachedLogger)

	config := cconf.NewConfigParamsFromTuples(
		"source", "test",
		"index", "log",
		"daily", true,
		"connection.host", host,
		"connection.port", port,
		"connection.protocol", "http",
	)
	logger.Configure(config)

	opnErr := logger.Open("")

	assert.Nil(t, opnErr)

	defer logger.Close("")

	t.Run("Log Level", fixture.TestLogLevel)
	t.Run("Simple Logging", fixture.TestSimpleLogging)
	t.Run("Error Logging", fixture.TestErrorLogging)

}
