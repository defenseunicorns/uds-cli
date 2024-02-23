package generate

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

type PackageSpec struct {
	Network struct {
		Allow []struct {
			Description     string            `json:"description,omitempty"`
			Direction       string            `json:"direction"`
			Labels          map[string]string `json:"labels,omitempty"`
			PodLabels       map[string]string `json:"podLabels,omitempty"`
			Port            int               `json:"port,omitempty"`
			Ports           []int             `json:"ports,omitempty"`
			RemoteGenerated string            `json:"remoteGenerated,omitempty"`
			RemoteNamespace string            `json:"remoteNamespace,omitempty"`
			RemotePodLabels map[string]string `json:"remotePodLabels,omitempty"`
			RemoteSelector  map[string]string `json:"remoteSelector,omitempty"`
			Selector        map[string]string `json:"selector,omitempty"`
		} `json:"allow,omitempty"`
		Expose []Expose `json:"expose,omitempty"`
	} `json:"network,omitempty"`
}

type Expose struct {
	AdvancedHTTP struct {
		CorsPolicy struct {
			AllowCredentials bool     `json:"allowCredentials,omitempty"`
			AllowHeaders     []string `json:"allowHeaders,omitempty"`
			AllowMethods     []string `json:"allowMethods,omitempty"`
			AllowOrigin      []string `json:"allowOrigin,omitempty"`
			AllowOrigins     []struct {
				Exact  string `json:"exact,omitempty"`
				Prefix string `json:"prefix,omitempty"`
				Regex  string `json:"regex,omitempty"`
			} `json:"allowOrigins,omitempty"`
			ExposeHeaders []string `json:"exposeHeaders,omitempty"`
			MaxAge        string   `json:"maxAge,omitempty"`
		} `json:"corsPolicy,omitempty"`
		DirectResponse struct {
			Body   map[string]string `json:"body,omitempty"`
			Status int               `json:"status,omitempty"`
		} `json:"directResponse,omitempty"`
		Headers struct {
			Request  map[string]string `json:"request,omitempty"`
			Response map[string]string `json:"response,omitempty"`
		} `json:"headers,omitempty"`
		Match []struct {
			IgnoreUriCase bool              `json:"ignoreUriCase,omitempty"`
			Method        map[string]string `json:"method,omitempty"`
			Name          string            `json:"name,omitempty"`
			QueryParams   map[string]string `json:"queryParams,omitempty"`
			Uri           map[string]string `json:"uri,omitempty"`
		} `json:"match,omitempty"`
		Retries struct {
			Attempts              int    `json:"attempts,omitempty"`
			PerTryTimeout         string `json:"perTryTimeout,omitempty"`
			RetryOn               string `json:"retryOn,omitempty"`
			RetryRemoteLocalities *bool  `json:"retryRemoteLocalities,omitempty"`
		} `json:"retries,omitempty"`
		Rewrite struct {
			Authority       string `json:"authority,omitempty"`
			Uri             string `json:"uri,omitempty"`
			UriRegexRewrite struct {
				Match   string `json:"match,omitempty"`
				Rewrite string `json:"rewrite,omitempty"`
			} `json:"uriRegexRewrite,omitempty"`
		} `json:"rewrite,omitempty"`
		Timeout string `json:"timeout,omitempty"`
		Weight  int    `json:"weight,omitempty"`
	} `json:"advancedHTTP,omitempty"`
	Description string `json:"description,omitempty"`
	Gateway     string `json:"gateway"`
	Host        string `json:"host"`
	Match       []struct {
		IgnoreUriCase bool              `json:"ignoreUriCase,omitempty"`
		Method        map[string]string `json:"method,omitempty"`
		Name          string            `json:"name,omitempty"`
		QueryParams   map[string]string `json:"queryParams,omitempty"`
		Uri           map[string]string `json:"uri,omitempty"`
	} `json:"match,omitempty"`
	PodLabels  map[string]string `json:"podLabels,omitempty"`
	Port       int               `json:"port"`
	Selector   map[string]string `json:"selector,omitempty"`
	Service    string            `json:"service"`
	TargetPort int               `json:"targetPort,omitempty"`
}

type PackageStatus struct {
	Endpoints          []string `json:"endpoints"`
	NetworkPolicyCount int      `json:"networkPolicyCount"`
	ObservedGeneration int      `json:"observedGeneration"`
	Phase              string   `json:"phase"`
}

type Package struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata"`

	Spec   PackageSpec   `json:"spec,omitempty"`
	Status PackageStatus `json:"status,omitempty"`
}

type PackageList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`

	Items []Package `json:"items"`
}
