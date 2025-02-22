/*
 * StorageOS API
 *
 * No description provided (generated by Openapi Generator https://github.com/openapitools/openapi-generator)
 *
 * API version: 2.5.0
 * Contact: info@storageos.com
 * Generated by: OpenAPI Generator (https://openapi-generator.tech)
 */

package api
// LogLevel This setting determines the log level for nodes across the cluster to use when recording entries in the log. All entries below the specified log level are discarded, where \"error\" is the highest log level and \"debug\" is the lowest. This setting is only checked by nodes on startup. Changing this setting will not affect the behaviour of nodes that are already operational. 
type LogLevel string

// List of LogLevel
const (
	LOGLEVEL_DEBUG LogLevel = "debug"
	LOGLEVEL_INFO LogLevel = "info"
	LOGLEVEL_WARN LogLevel = "warn"
	LOGLEVEL_ERROR LogLevel = "error"
)
