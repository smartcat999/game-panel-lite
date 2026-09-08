package terraria

import (
	"errors"
	"strings"
)

func (p VanillaProvider) NormalizeLogicalConfig(input map[string]any) (map[string]any, error) {
	return logicalConfig(input, p.DefaultConfig())
}
func (p TModLoaderProvider) NormalizeLogicalConfig(input map[string]any) (map[string]any, error) {
	return logicalConfig(input, p.DefaultConfig())
}

func logicalConfig(input map[string]any, defaults Config) (map[string]any, error) {
	for key, value := range input {
		switch key {
		case "serverName", "worldName", "worldSize", "worldEvil", "difficulty", "maxPlayers", "password", "motd", "seed", "specialSeeds", "secretSeeds", "secure", "language", "autoCreateWorld":
		default:
			return nil, errors.New("unsupported logical Terraria configuration field")
		}
		if value == nil {
			return nil, errors.New("null logical Terraria configuration field")
		}
	}
	config, err := terrariaConfigFromPayload(input, defaults)
	if err != nil {
		return nil, err
	}
	if err := ValidateConfig(config); err != nil {
		return nil, err
	}
	stringsToCheck := []string{config.ServerName, config.WorldName, config.Password, config.MOTD, config.Seed, config.Language}
	stringsToCheck = append(stringsToCheck, config.SpecialSeeds...)
	stringsToCheck = append(stringsToCheck, config.SecretSeeds...)
	for _, value := range stringsToCheck {
		if strings.ContainsAny(value, "\r\n\x00") {
			return nil, errors.New("multiline logical Terraria configuration is unsupported")
		}
	}
	result := terrariaPayloadFromConfig(config)
	delete(result, "port")
	return result, nil
}
