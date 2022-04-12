package storageos

const (
	// Node container env vars
	volumeLockTTLEnvVar       = "VOLUME_LOCK_TTL"
	healthProbeIntervalEnvVar = "HEALTH_PROBE_INTERVAL"

	// Node failover policy names
	policyDefault  = ""
	policyTolerant = "tolerant"
	policyStrict   = "strict"
)

var defaultEnvVars = map[string]string{
	volumeLockTTLEnvVar:       "",
	healthProbeIntervalEnvVar: "",
}

var tolerantEnvVars = map[string]string{
	volumeLockTTLEnvVar:       "10s",
	healthProbeIntervalEnvVar: "5s",
}

var strictEnvVars = map[string]string{
	volumeLockTTLEnvVar:       "5s",
	healthProbeIntervalEnvVar: "1s",
}

// FailoverPolicyEnvVars returns a pre-defined map of env vars based on
// the policy name.
func FailoverPolicyEnvVars(name string) map[string]string {
	switch name {
	case policyDefault:
		return defaultEnvVars
	case policyTolerant:
		return tolerantEnvVars
	case policyStrict:
		return strictEnvVars
	default:
		return nil
	}
}
