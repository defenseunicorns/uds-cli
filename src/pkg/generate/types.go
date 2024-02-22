package generate

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

type PackageSpec struct {
	Network struct {
		Allow []struct {
			Description     string            `json:"description"`
			Direction       string            `json:"direction"`
			Labels          map[string]string `json:"labels"`
			PodLabels       map[string]string `json:"podLabels"`
			Port            int               `json:"port"`
			Ports           []int             `json:"ports"`
			RemoteGenerated string            `json:"remoteGenerated"`
			RemoteNamespace string            `json:"remoteNamespace"`
			RemotePodLabels map[string]string `json:"remotePodLabels"`
			RemoteSelector  map[string]string `json:"remoteSelector"`
			Selector        map[string]string `json:"selector"`
		} `json:"allow"`
		Expose []struct {
			AdvancedHTTP struct {
				CorsPolicy struct {
					AllowCredentials bool     `json:"allowCredentials"`
					AllowHeaders     []string `json:"allowHeaders"`
					AllowMethods     []string `json:"allowMethods"`
					AllowOrigin      []string `json:"allowOrigin"`
					AllowOrigins     []struct {
						Exact  string `json:"exact,omitempty"`
						Prefix string `json:"prefix,omitempty"`
						Regex  string `json:"regex,omitempty"`
					} `json:"allowOrigins,omitempty"`
					ExposeHeaders []string `json:"exposeHeaders"`
					MaxAge        string   `json:"maxAge"`
				} `json:"corsPolicy"`
				DirectResponse struct {
					Body   map[string]string `json:"body,omitempty"`
					Status int               `json:"status"`
				} `json:"directResponse,omitempty"`
				Headers struct {
					Request  map[string]string `json:"request,omitempty"`
					Response map[string]string `json:"response,omitempty"`
				} `json:"headers"`
				Match []struct {
					IgnoreUriCase bool              `json:"ignoreUriCase"`
					Method        map[string]string `json:"method"`
					Name          string            `json:"name"`
					QueryParams   map[string]string `json:"queryParams"`
					Uri           map[string]string `json:"uri"`
				} `json:"match"`
				Retries struct {
					Attempts              int    `json:"attempts"`
					PerTryTimeout         string `json:"perTryTimeout"`
					RetryOn               string `json:"retryOn"`
					RetryRemoteLocalities *bool  `json:"retryRemoteLocalities,omitempty"`
				} `json:"retries"`
				Rewrite struct {
					Authority       string `json:"authority"`
					Uri             string `json:"uri"`
					UriRegexRewrite struct {
						Match   string `json:"match"`
						Rewrite string `json:"rewrite"`
					} `json:"uriRegexRewrite"`
				} `json:"rewrite"`
				Timeout string `json:"timeout"`
				Weight  int    `json:"weight"`
			} `json:"advancedHTTP"`
			Description string `json:"description"`
			Gateway     string `json:"gateway"`
			Host        string `json:"host"`
			Match       []struct {
				IgnoreUriCase bool              `json:"ignoreUriCase"`
				Method        map[string]string `json:"method"`
				Name          string            `json:"name"`
				QueryParams   map[string]string `json:"queryParams"`
				Uri           map[string]string `json:"uri"`
			} `json:"match"`
			PodLabels  map[string]string `json:"podLabels"`
			Port       int               `json:"port"`
			Selector   map[string]string `json:"selector"`
			Service    string            `json:"service"`
			TargetPort int               `json:"targetPort"`
		} `json:"expose"`
	} `json:"network"`
}

type PackageStatus struct {
	Endpoints          []string `json:"endpoints"`
	NetworkPolicyCount int      `json:"networkPolicyCount"`
	ObservedGeneration int      `json:"observedGeneration"`
	Phase              string   `json:"phase"`
}

type Package struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   PackageSpec   `json:"spec,omitempty"`
	Status PackageStatus `json:"status,omitempty"`
}

type PackageList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`

	Items []Package `json:"items"`
}
