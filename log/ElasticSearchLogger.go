package log

import (
	"net/http"
	"time"

	esv8 "github.com/elastic/go-elasticsearch/v8"
	cconf "github.com/pip-services3-go/pip-services3-commons-go/config"
	cerr "github.com/pip-services3-go/pip-services3-commons-go/errors"
	cref "github.com/pip-services3-go/pip-services3-commons-go/refer"
	clog "github.com/pip-services3-go/pip-services3-components-go/log"
	crpccon "github.com/pip-services3-go/pip-services3-rpc-go/connect"
)

/*
Logger that dumps execution logs to ElasticSearch service.

ElasticSearch is a popular search index. It is often used
to store and index execution logs by itself or as a part of
ELK (ElasticSearch - Logstash - Kibana) stack.

Authentication is not supported in this version.

 Configuration parameters

- level:             maximum log level to capture
- source:            source (context) name
- connection(s):
    - discovery_key:         (optional) a key to retrieve the connection from [[https://rawgit.com/pip-services-node/pip-services3-components-node/master/doc/api/interfaces/connect.idiscovery.html IDiscovery]]
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

 References

- *:context-info:*:*:1.0      (optional)  ContextInfo to detect the context id and specify counters source
- *:discovery:*:*:1.0         (optional)  IDiscovery services to resolve connection

 Example

    let logger = new ElasticSearchLogger();
    logger.configure(ConfigParams.fromTuples(
        "connection.protocol", "http",
        "connection.host", "localhost",
        "connection.port", 9200
    ));

    logger.open("123", (err) => {
        ...
    });

    logger.error("123", ex, "Error occured: %s", ex.message);
    logger.debug("123", "Everything is OK.");
*/
// implements IReferenceable, IOpenable
type ElasticSearchLogger struct {
	clog.CachedLogger
	connectionResolver *crpccon.HttpConnectionResolver

	timer        chan bool
	index        string
	dailyIndex   bool
	currentIndex string
	reconnect    int
	timeout      int
	maxRetries   int
	indexMessage bool
	interval     int

	client *esv8.Client
}

/*
   Creates a new instance of the logger.
*/
func NewElasticSearchLogger() *ElasticSearchLogger {
	esl := ElasticSearchLogger{}
	//esl.CachedLogger = clog.NewCachedLogger()
	esl.connectionResolver = crpccon.NewHttpConnectionResolver()
	esl.index = "log"
	esl.dailyIndex = false
	esl.reconnect = 60000
	esl.timeout = 30000
	esl.maxRetries = 3
	esl.interval = 10000
	esl.indexMessage = false
	return &esl
}

/*
Configures component by passing configuration parameters.

- config    configuration parameters to be set.
*/
func (c *ElasticSearchLogger) Configure(config *cconf.ConfigParams) {
	c.CachedLogger.Configure(config)

	c.connectionResolver.Configure(config)

	c.index = config.GetAsStringWithDefault("index", c.index)
	c.dailyIndex = config.GetAsBooleanWithDefault("daily", c.dailyIndex)
	c.reconnect = config.GetAsIntegerWithDefault("options.reconnect", c.reconnect)
	c.timeout = config.GetAsIntegerWithDefault("options.timeout", c.timeout)
	c.maxRetries = config.GetAsIntegerWithDefault("options.max_retries", c.maxRetries)
	c.indexMessage = config.GetAsBooleanWithDefault("options.index_message", c.indexMessage)

	c.interval = config.GetAsIntegerWithDefault("options.interval", c.interval)
}

/*
Sets references to dependent components.

- references 	references to locate the component dependencies.
*/
func (c *ElasticSearchLogger) SetReferences(references cref.IReferences) {
	c.CachedLogger.SetReferences(references)
	c.connectionResolver.SetReferences(references)
}

/*
Checks if the component is opened.
Returns true if the component has been opened and false otherwise.
*/
func (c *ElasticSearchLogger) IsOpen() bool {
	return c.timer != nil
}

/*
Opens the component.

- correlationId 	(optional) transaction id to trace execution through call chain.
- callback 			callback function that receives error or nil no errors occured.
*/
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
		c.timer = setInterval(func() { c.Dump() }, c.interval, true)
	}

	return nil
}

/*
Closes component and frees used resources.

- correlationId 	(optional) transaction id to trace execution through call chain.
- callback 			callback function that receives error or nil no errors occured.
*/
func (c *ElasticSearchLogger) Close(correlationId string) (err error) {
	svErr := c.Save(c.Cache)
	if svErr == nil {
		return svErr
	}

	if c.timer != nil {
		c.timer <- true
	}

	c.Cache = nil

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
	//     c.client.indices.exists(
	//         { index: c.currentIndex },
	//         (err, exists) => {
	//             if (err || exists) {
	//                 callback(err);
	//                 return;
	//             }

	//             c.client.indices.create(
	//                 {
	//                     index: c.currentIndex,
	//                     body: {
	//                         settings: {
	//                             int_of_shards: 1
	//                         },
	//                         mappings: {
	//                             log_message: {
	//                                 properties: {
	//                                     time: { type: "date", index: true },
	//                                     source: { type: "keyword", index: true },
	//                                     level: { type: "keyword", index: true },
	//                                     correlation_id: { type: "text", index: true },
	//                                     error: {
	//                                         type: "object",
	//                                         properties: {
	//                                             type: { type: "keyword", index: true },
	//                                             category: { type: "keyword", index: true },
	//                                             status: { type: "integer", index: false },
	//                                             code: { type: "keyword", index: true },
	//                                             message: { type: "text", index: false },
	//                                             details: { type: "object" },
	//                                             correlation_id: { type: "text", index: false },
	//                                             cause: { type: "text", index: false },
	//                                             stack_trace: { type: "text", index: false }
	//                                         }
	//                                     },
	//                                     message: { type: "text", index: c.indexMessage }
	//                                 }
	//                             }
	//                         }
	//                     }
	//                 },
	//                 (err) => {
	//                     // Skip already exist errors
	//                     if (err && err.message.indexOf("resource_already_exists") >= 0)
	//                         err = nil;

	//                     callback(err);
	//                 }
	//             );
	//         }
	//     );
	return nil
}

/*
Saves log messages from the cache.

- messages  a list with log messages
- callback  callback function that receives error or nil for success.
*/
func (c *ElasticSearchLogger) Save(messages []clog.LogMessage) (err error) {

	if !c.IsOpen() && len(messages) == 0 {
		return nil
	}

	err = c.createIndexIfNeeded("elasticsearch_logger", false)

	if err != nil {
		return nil
	}

	// let bulk = [];
	// for (let message of messages) {
	//     bulk.push({ index: { index: c.currentIndex, _type: "log_message", _id: IdGenerator.nextLong() } })
	//     bulk.push(message);
	// }
	// c.client.Bulk({ body: bulk }, callback);
	return err
}

func setInterval(someFunc func(), milliseconds int, async bool) chan bool {

	// How often to fire the passed in function
	// in milliseconds
	interval := time.Duration(milliseconds) * time.Millisecond

	// Setup the ticket and the channel to signal
	// the ending of the interval
	ticker := time.NewTicker(interval)
	clear := make(chan bool)

	// Put the selection in a go routine
	// so that the for loop is none blocking
	go func() {
		for {

			select {
			case <-ticker.C:
				if async {
					// This won't block
					go someFunc()
				} else {
					// This will block
					someFunc()
				}
			case <-clear:
				ticker.Stop()
				return
			}

		}
	}()

	// We return the channel so we can pass in
	// a value to it to clear the interval
	return clear
}
