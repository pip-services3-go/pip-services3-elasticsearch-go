package log

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	esv8 "github.com/elastic/go-elasticsearch/v8"
	cconf "github.com/pip-services3-go/pip-services3-commons-go/config"
	cdata "github.com/pip-services3-go/pip-services3-commons-go/data"
	cerr "github.com/pip-services3-go/pip-services3-commons-go/errors"
	cref "github.com/pip-services3-go/pip-services3-commons-go/refer"
	clog "github.com/pip-services3-go/pip-services3-components-go/log"
	crpccon "github.com/pip-services3-go/pip-services3-rpc-go/connect"
)

/*
ElasticSearchLogger is logger that dumps execution logs to ElasticSearch service.
ElasticSearch is a popular search index. It is often used
to store and index execution logs by itself or as a part of
ELK (ElasticSearch - Logstash - Kibana) stack.

Authentication is not supported in this version.

Configuration parameters:

- level:             maximum log level to capture
- source:            source (context) name
- connection(s):
    - discovery_key:         (optional) a key to retrieve the connection from IDiscovery
    - protocol:              connection protocol: http or https
    - host:                  host name or IP address
    - port:                  port int
    - uri:                   resource URI or connection string with all parameters in it
- options:
    - interval:        interval in milliseconds to save log messages (default: 10 seconds)
    - max_cache_size:  maximum int of messages stored in this cache (default: 100)
    - index:           ElasticSearch index name (default: "log")
    - daily:           true to create a new index every day by adding date suffix to the index
                       name (default: false)
    - reconnect:       reconnect timeout in milliseconds (default: 60 sec)
    - timeout:         invocation timeout in milliseconds (default: 30 sec)
    - max_retries:     maximum int of retries (default: 3)
    - index_message:   true to enable indexing for message object (default: false)

References:

- *:context-info:*:*:1.0      (optional)  ContextInfo to detect the context id and specify counters source
- *:discovery:*:*:1.0         (optional)  IDiscovery services to resolve connection

Example:

    logger := NewElasticSearchLogger();
    logger.Configure(cconf.NewConfigParamsFromTuples(
        "connection.protocol", "http",
        "connection.host", "localhost",
		"connection.port", "9200"
    ));

    logger.Open("123")

    logger.Error("123", ex, "Error occured: %s", ex.message);
    logger.Debug("123", "Everything is OK.");
*/
type ElasticSearchLogger struct {
	*clog.CachedLogger
	connectionResolver *crpccon.HttpConnectionResolver

	timer        chan bool
	index        string
	dailyIndex   bool
	currentIndex string
	reconnect    int
	timeout      int
	maxRetries   int
	indexMessage bool

	client *esv8.Client
}

// NewElasticSearchLogger method creates a new instance of the logger.
// Retruns *ElasticSearchLogger
// pointer on new ElasticSearchLogger
func NewElasticSearchLogger() *ElasticSearchLogger {
	c := ElasticSearchLogger{}
	c.CachedLogger = clog.InheritCachedLogger(&c)
	c.connectionResolver = crpccon.NewHttpConnectionResolver()
	c.index = "log"
	c.dailyIndex = false
	c.reconnect = 60000
	c.timeout = 30000
	c.maxRetries = 3
	c.Interval = 10000
	c.indexMessage = false
	return &c
}

// Configure are configures component by passing configuration parameters.
// Parameters:
// 	- config  *cconf.ConfigParams   configuration parameters to be set.
func (c *ElasticSearchLogger) Configure(config *cconf.ConfigParams) {
	c.CachedLogger.Configure(config)

	c.connectionResolver.Configure(config)

	c.index = config.GetAsStringWithDefault("index", c.index)
	c.dailyIndex = config.GetAsBooleanWithDefault("daily", c.dailyIndex)
	c.reconnect = config.GetAsIntegerWithDefault("options.reconnect", c.reconnect)
	c.timeout = config.GetAsIntegerWithDefault("options.timeout", c.timeout)
	c.maxRetries = config.GetAsIntegerWithDefault("options.max_retries", c.maxRetries)
	c.indexMessage = config.GetAsBooleanWithDefault("options.index_message", c.indexMessage)
}

// SetReferences method are sets references to dependent components.
// Parameters:
// 	- references cref.IReferences 	references to locate the component dependencies.
func (c *ElasticSearchLogger) SetReferences(references cref.IReferences) {
	c.CachedLogger.SetReferences(references)
	c.connectionResolver.SetReferences(references)
}

// IsOpen method are checks if the component is opened.
// Returns true if the component has been opened and false otherwise.
func (c *ElasticSearchLogger) IsOpen() bool {
	return c.timer != nil
}

// Open method are ppens the component.
// Parameters:
// - correlationId string 	(optional) transaction id to trace execution through call chain.
// Returns error or nil, if no errors occured.
func (c *ElasticSearchLogger) Open(correlationId string) (err error) {
	if c.IsOpen() {
		return nil
	}

	connection, _, err := c.connectionResolver.Resolve(correlationId)

	if connection == nil {
		err = cerr.NewConfigError(correlationId, "NO_CONNECTION", "Connection is not configured")
	}

	if err != nil {
		return err
	}

	uri := connection.Uri()

	options := esv8.Config{
		Addresses: []string{uri},
		Transport: &http.Transport{
			ResponseHeaderTimeout: (time.Duration)(c.timeout) * time.Millisecond,
			IdleConnTimeout:       (time.Duration)(c.reconnect) * time.Millisecond},
		MaxRetries: c.maxRetries,
	}

	elasticsearch, esErr := esv8.NewClient(options)
	if esErr != nil {
		return esErr
	}
	c.client = elasticsearch

	err = c.createIndexIfNeeded(correlationId, true)
	if err == nil {
		c.timer = setInterval(func() { c.Dump() }, c.Interval, true)
	}

	return nil
}

// Close method are closes component and frees used resources.
// Parameters:
// - correlationId  string	(optional) transaction id to trace execution through call chain.
// Returns error or nil, if no errors occured.
func (c *ElasticSearchLogger) Close(correlationId string) (err error) {
	svErr := c.Save(c.Cache)
	if svErr == nil {
		return svErr
	}

	if c.timer != nil {
		c.timer <- true
	}

	c.Cache = make([]*clog.LogMessage, 0, 0)

	c.timer = nil
	c.client = nil
	return nil
}

func (c *ElasticSearchLogger) getCurrentIndex() string {
	if !c.dailyIndex {
		return c.index
	}
	now := time.Now()
	return c.index + "-" + now.UTC().Format("20060102")
}

func (c *ElasticSearchLogger) createIndexIfNeeded(correlationId string, force bool) (err error) {
	newIndex := c.getCurrentIndex()
	if !force && c.currentIndex == newIndex {
		return nil
	}

	c.currentIndex = newIndex
	exists, err := c.client.Indices.Exists([]string{c.currentIndex})
	if err != nil || exists.StatusCode == 404 {
		return err
	}

	indBody := `{
		"settings": {
			"number_of_shards": "1"
		},
		"mappings": {
			"log_message": {
				"properties": {
					"time": { "type": "date", "index": true },
					"source": { "type": "keyword", "index": true },
					"level": { "type": "keyword", "index": true },
					"correlation_id": { "type": "text", "index": true },
					"error": {
						"type": "object",
						"properties": {
							"type": { "type": "keyword", "index": true },
							"category": { "type": "keyword", "index": true },
							"status": { "type": "integer", "index": false },
							"code": { "type": "keyword", "index": true },
							"message": { "type": "text", "index": false },
							"details": { "type": "object" },
							"correlation_id": { "type": "text", "index": false },
							"cause": { "type": "text", "index": false },
							"stack_trace": { "type": "text", "index": false }
						}
					},
					"message": { "type": "text", "index":` + strconv.FormatBool(c.indexMessage) + ` }
				}
			}
		}
	}`

	resp, err := c.client.Indices.Create(c.currentIndex,
		c.client.Indices.Create.WithBody(strings.NewReader(indBody)),
	)
	if resp != nil {
		defer resp.Body.Close()
	}

	if err != nil {
		return err
	}

	if resp.IsError() {
		var e map[string]interface{}
		if err = json.NewDecoder(resp.Body).Decode(&e); err != nil {
			return err
		}
		// Skip already exist errors
		if strings.Index(e["error"].(map[string]interface{})["type"].(string), "resource_already_exists") >= 0 {
			return nil
		}
		err = cerr.NewError(e["error"].(map[string]interface{})["type"].(string)).WithCauseString(e["error"].(map[string]interface{})["reason"].(string))
	}
	return nil
}

// Save method are saves log messages from the cache.
// Parameters:
// - messages []*clog.LogMessage a list with log messages
// Retruns error or nil for success.
func (c *ElasticSearchLogger) Save(messages []*clog.LogMessage) (err error) {

	if !c.IsOpen() || len(messages) == 0 {
		return nil
	}

	err = c.createIndexIfNeeded("elasticsearch_logger", false)

	if err != nil {
		return nil
	}

	var buf bytes.Buffer
	for _, message := range messages {
		meta := []byte(fmt.Sprintf(`{ "index": { "_index":"%s", "_type":"log_message", "_id":"%s"}}%s`, c.currentIndex, cdata.IdGenerator.NextLong(), "\n"))
		data, err := json.Marshal(message)

		if err != nil {
			c.Logger.Error("", err, "Cannot encode message "+err.Error())
		}
		data = append(data, "\n"...)
		buf.Grow(len(meta) + len(data))
		buf.Write(meta)
		buf.Write(data)
	}

	resp, err := c.client.Bulk(bytes.NewReader(buf.Bytes()), c.client.Bulk.WithIndex(c.currentIndex))
	if err != nil {
		c.Logger.Error("", err, "Failure indexing batch %s", err.Error())
	}
	if resp != nil {
		defer resp.Body.Close()
	}
	buf.Reset()

	if resp.IsError() {
		var e map[string]interface{}
		if err = json.NewDecoder(resp.Body).Decode(&e); err != nil {
			return err
		}
		err = cerr.NewError(e["error"].(map[string]interface{})["type"].(string)).WithCauseString(e["error"].(map[string]interface{})["reason"].(string))
	}
	return err
}

func setInterval(someFunc func(), milliseconds int, async bool) chan bool {

	interval := time.Duration(milliseconds) * time.Millisecond
	ticker := time.NewTicker(interval)
	clear := make(chan bool)
	go func() {
		for {
			select {
			case <-ticker.C:
				if async {
					go someFunc()
				} else {
					someFunc()
				}
			case <-clear:
				ticker.Stop()
				return
			}

		}
	}()

	return clear
}
