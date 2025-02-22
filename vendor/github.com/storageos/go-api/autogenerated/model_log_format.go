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
// LogFormat This setting determines the format nodes in the cluster will use for log entries. This setting is only checked by nodes on startup. Changing this setting will not affect the behaviour of nodes that are already operational. 
type LogFormat string

// List of LogFormat
const (
	LOGFORMAT_DEFAULT LogFormat = "default"
	LOGFORMAT_JSON LogFormat = "json"
)
