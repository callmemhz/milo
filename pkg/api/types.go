package api

type CreateUserReq struct {
	Username string `json:"username"`
	IsAdmin  bool   `json:"is_admin"`
}

type UserResp struct {
	ID       int64  `json:"id"`
	Username string `json:"username"`
	IsAdmin  bool   `json:"is_admin"`
}

type CreateTokenReq struct {
	Name string `json:"name"`
}

type CreateTokenResp struct {
	ID    int64  `json:"id"`
	Token string `json:"token"` // plaintext; returned ONCE
	Name  string `json:"name"`
}

type TokenResp struct {
	ID         int64  `json:"id"`
	Name       string `json:"name,omitempty"`
	Kind       string `json:"kind"`
	LastUsedAt string `json:"last_used_at,omitempty"`
}

type CreateAppReq struct {
	Name             string   `json:"name"`
	Port             int64    `json:"port,omitempty"`
	HealthPath       string   `json:"health_path,omitempty"`
	HealthTimeoutSec int64    `json:"health_timeout_sec,omitempty"`
	CPULimit         float64  `json:"cpu_limit,omitempty"`
	MemoryLimitMB    int64    `json:"memory_limit_mb,omitempty"`
	Owners           []string `json:"owners,omitempty"`
}

type UpdateAppReq struct {
	Port             *int64    `json:"port,omitempty"`
	HealthPath       *string   `json:"health_path,omitempty"`
	HealthTimeoutSec *int64    `json:"health_timeout_sec,omitempty"`
	CPULimit         *float64  `json:"cpu_limit,omitempty"`
	MemoryLimitMB    *int64    `json:"memory_limit_mb,omitempty"`
	Owners           *[]string `json:"owners,omitempty"`
}

type AppResp struct {
	ID               int64    `json:"id"`
	Name             string   `json:"name"`
	Port             int64    `json:"port"`
	HealthPath       string   `json:"health_path"`
	HealthTimeoutSec int64    `json:"health_timeout_sec"`
	CPULimit         float64  `json:"cpu_limit"`
	MemoryLimitMB    int64    `json:"memory_limit_mb"`
	Owners           []string `json:"owners"`
}

type EnvPatchReq struct {
	Set   map[string]string `json:"set,omitempty"`
	Unset []string          `json:"unset,omitempty"`
}

type CreateDeploymentReq struct {
	Image  string `json:"image"`
	Commit string `json:"commit,omitempty"`
	Ref    string `json:"ref,omitempty"`
}

type DeploymentResp struct {
	ID            int64  `json:"id"`
	AppID         int64  `json:"app_id"`
	ImageDigest   string `json:"image_digest,omitempty"`
	ImageRef      string `json:"image_ref"`
	Status        string `json:"status"`
	FailureReason string `json:"failure_reason,omitempty"`
	ContainerName string `json:"container_name,omitempty"`
	Commit        string `json:"commit,omitempty"`
	Ref           string `json:"ref,omitempty"`
	CreatedAt     string `json:"created_at"`
	FinishedAt    string `json:"finished_at,omitempty"`
}
