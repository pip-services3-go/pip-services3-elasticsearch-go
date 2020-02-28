package build

import (
	cref "github.com/pip-services3-go/pip-services3-commons-go/refer"
	cbuild "github.com/pip-services3-go/pip-services3-components-go/build"
	elog "github.com/pip-services3-go/pip-services3-elasticsearch-go/log"
)

/*
DefaultElasticSearchFactory are creates ElasticSearch components by their descriptors.
See ElasticSearchLogger
*/
type DefaultElasticSearchFactory struct {
	cbuild.Factory
	Descriptor                    *cref.Descriptor
	ElasticSearchLoggerDescriptor *cref.Descriptor
}

// NewDefaultElasticSearchFactory create a new instance of the factory.
// Retruns *DefaultElasticSearchFactory
// pointer on new factory
func NewDefaultElasticSearchFactory() *DefaultElasticSearchFactory {
	c := DefaultElasticSearchFactory{}
	c.Factory = *cbuild.NewFactory()
	c.Descriptor = cref.NewDescriptor("pip-services", "factory", "elasticsearch", "default", "1.0")
	c.ElasticSearchLoggerDescriptor = cref.NewDescriptor("pip-services", "logger", "elasticsearch", "*", "1.0")
	c.RegisterType(c.ElasticSearchLoggerDescriptor, elog.NewElasticSearchLogger)
	return &c
}
