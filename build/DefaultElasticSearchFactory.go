package build

import (
	cref "github.com/pip-services3-go/pip-services3-commons-go/refer"
	cbuild "github.com/pip-services3-go/pip-services3-components-go/build"
	elog "github.com/pip-services3-node/pip-services3-elasticsearch-go/log"
)

/*
Creates ElasticSearch components by their descriptors.

SeeElasticSearchLogger
*/
type DefaultElasticSearchFactory struct {
	cbuild.Factory
	Descriptor                    *cref.Descriptor
	ElasticSearchLoggerDescriptor *cref.Descriptor
}

/*
	Create a new instance of the factory.
*/
func NewDefaultElasticSearchFactory() *DefaultElasticSearchFactory {
	//super();
	desf := DefaultElasticSearchFactory{}
	desf.Factory = *cbuild.NewFactory()
	desf.Descriptor = cref.NewDescriptor("pip-services", "factory", "elasticsearch", "default", "1.0")
	desf.ElasticSearchLoggerDescriptor = cref.NewDescriptor("pip-services", "logger", "elasticsearch", "*", "1.0")
	desf.RegisterType(desf.ElasticSearchLoggerDescriptor, elog.NewElasticSearchLogger)
	return &desf
}
