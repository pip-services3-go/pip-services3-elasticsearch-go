# Configuration Guide <br/>

Configuration structure follows the 
[standard configuration](https://github.com/pip-services/pip-services3-container-node/doc/Configuration.md) 
structure. 

### <a name="log_elasticsearch"></a> Elasticsearch

Elasticsearch logger has the following configuration properties:
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

Example:
```yaml
- descriptor: "pip-services-clusters:log:elasticsearch:default:1.0"
  source: "test"
  connection:
    protocol: "http"
    host: "localhost"
    port: 9200
  options:
    interval: 10
    max_cache_size: 100
    index: "log"
    daily: true       
```

For more information on this section read 
[Pip.Services Configuration Guide](https://github.com/pip-services/pip-services3-container-node/doc/Configuration.md#deps)