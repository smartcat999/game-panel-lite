package instances

// ConfigurationBinding is immutable resource identity authenticated alongside
// ciphertext. Region is deliberately absent: a logical revision can migrate.
type ConfigurationBinding struct {
	OrganizationID      string `json:"organizationId"`
	ServerID            string `json:"serverId"`
	RevisionID          string `json:"revisionId"`
	SpecGeneration      int64  `json:"specGeneration"`
	ProviderKey         string `json:"providerKey"`
	ConfigSchemaVersion int    `json:"configSchemaVersion"`
}

func (b ConfigurationBinding) Validate() error {
	if !identifier(b.OrganizationID) || !identifier(b.ServerID) || !identifier(b.RevisionID) || !identifier(b.ProviderKey) || b.SpecGeneration < 1 || b.ConfigSchemaVersion < 1 {
		return ErrInvalidIntent
	}
	return nil
}
