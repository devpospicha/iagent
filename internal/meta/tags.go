package meta

import (
	"fmt"
	"strings"
	"time"

	"github.com/devpospicha/ishared/model"
)

// BuildStandardTags sets required labels for consistent metric identity and filtering.
// It sets the "namespace" and "job" labels, which are used to identify the source of the metric.
func BuildStandardTags(meta *model.Meta, m model.Metric, isContainer bool, startTime time.Time) {
	if meta.Tags == nil {
		meta.Tags = make(map[string]string)
	}
	// Tag with agent start time for use calculating agent uptime on server
	meta.Tags["agent_start_time"] = fmt.Sprintf("%d", startTime.Unix())

	// Contextual source of the metric
	meta.Tags["namespace"] = strings.ToLower(m.Namespace)
	//meta.Tags["subnamespace"] = strings.ToLower(m.SubNamespace)

	// Producer of metric becomes the "job"
	if isContainer {
		meta.Tags["job"] = "gosight-container"

		if meta.ContainerName != "" {
			meta.Tags["instance"] = meta.ContainerName

		} else if meta.ContainerID != "" {
			meta.Tags["container_id"] = meta.ContainerID

		} else {
			meta.Tags["instance"] = "unknown-container"
		}
	} else {
		meta.Tags["job"] = "gosight-agent"
		meta.Tags["instance"] = meta.Hostname

	}

}
