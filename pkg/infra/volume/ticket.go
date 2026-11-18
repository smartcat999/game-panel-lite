package volume

import (
	"crypto/subtle"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/golang-jwt/jwt/v5"

	corev1 "github.com/smartcat999/game-panel-cloud/pkg/apis/core/v1"
	metav1 "github.com/smartcat999/game-panel-cloud/pkg/apis/meta/v1"
)

// FileTicketClaims defines the JWT-signed payload authorizing a short-lived
// direct client-to-node-agent volume file stream without routing bulk traffic through apiserver.
type FileTicketClaims struct {
	InstanceUID      string `json:"uid,omitempty"`
	NodeUID          string `json:"nodeUID,omitempty"`
	AttemptID        string `json:"attemptID,omitempty"`
	Epoch            int64  `json:"epoch,omitempty"`
	ApprovedRevision int64  `json:"revision,omitempty"`
	Tenant           string `json:"ten"`
	Instance         string `json:"ins"`
	NodeName         string `json:"nod"`
	Action           string `json:"act"` // list | read | write | mkdir | rename | delete | backup | restore
	Path             string `json:"pth,omitempty"`
	TargetPath       string `json:"tpt,omitempty"`
	SourcePath       string `json:"spt,omitempty"`
	ReplaceTarget    bool   `json:"rpl,omitempty"`
	PresignedURL     string `json:"url,omitempty"`
	MaxBytes         int64  `json:"max,omitempty"`
	RateLimit        int64  `json:"rat,omitempty"`
	ExpiresAt        int64  `json:"exp"`
}

type fileTicketJWTClaims struct {
	Payload FileTicketClaims `json:"pld"`
	jwt.RegisteredClaims
}

// SignFileTicket signs FileTicketClaims as a compact HS256 JWT token using github.com/golang-jwt/jwt/v5.
func SignFileTicket(secret string, claims FileTicketClaims) (string, error) {
	if secret == "" {
		return "", errors.New("file ticket secret is required")
	}
	jwtClaims := fileTicketJWTClaims{
		Payload: claims,
		RegisteredClaims: jwt.RegisteredClaims{
			ExpiresAt: jwt.NewNumericDate(time.Unix(claims.ExpiresAt, 0)),
		},
	}
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, jwtClaims)
	return token.SignedString([]byte(secret))
}

// VerifyFileTicket validates the HS256 JWT signature and expiration timestamp of a file ticket.
func VerifyFileTicket(secret, tokenStr string, now time.Time) (*FileTicketClaims, error) {
	if secret == "" || tokenStr == "" {
		return nil, errors.New("missing ticket or secret")
	}
	var parsed fileTicketJWTClaims
	parser := jwt.NewParser(
		jwt.WithValidMethods([]string{jwt.SigningMethodHS256.Alg()}),
		jwt.WithStrictDecoding(),
		jwt.WithTimeFunc(func() time.Time { return now }),
	)
	token, err := parser.ParseWithClaims(tokenStr, &parsed, func(_ *jwt.Token) (any, error) {
		return []byte(secret), nil
	})
	if err != nil || !token.Valid {
		return nil, fmt.Errorf("invalid file ticket: %w", err)
	}
	if parsed.Payload.ExpiresAt <= now.Unix() {
		return nil, errors.New("file ticket expired")
	}
	return &parsed.Payload, nil
}

// AllowsMethodAndAction checks whether the signed ticket action permits the HTTP method and sub-action.
func (c *FileTicketClaims) AllowsMethodAndAction(httpMethod, subAction string) error {
	if c == nil {
		return errors.New("missing file ticket claims")
	}
	act := strings.ToLower(strings.TrimSpace(c.Action))
	switch httpMethod {
	case http.MethodGet:
		if act == corev1.FileActionList || act == corev1.FileActionRead {
			return nil
		}
		return fmt.Errorf("ticket action %q does not allow GET", c.Action)
	case http.MethodPut:
		if act == corev1.FileActionWrite {
			return nil
		}
		return fmt.Errorf("ticket action %q does not allow PUT", c.Action)
	case http.MethodDelete:
		if act == corev1.FileActionDelete {
			return nil
		}
		return fmt.Errorf("ticket action %q does not allow DELETE", c.Action)
	case http.MethodPost:
		sub := strings.ToLower(strings.TrimSpace(subAction))
		if (sub == corev1.FileActionMkdir && act == corev1.FileActionMkdir) ||
			(sub == corev1.FileActionRename && act == corev1.FileActionRename) ||
			(sub == corev1.FileActionBackup && act == corev1.FileActionBackup) ||
			(sub == corev1.FileActionRestore && act == corev1.FileActionRestore) {
			return nil
		}
		return fmt.Errorf("ticket action %q does not allow POST action %q", c.Action, subAction)
	default:
		return fmt.Errorf("unsupported HTTP method %q", httpMethod)
	}
}

// AllowsPath verifies that reqPath falls within the path authorized by the signed ticket.
func (c *FileTicketClaims) AllowsPath(reqPath string, allowRoot bool) error {
	if c == nil {
		return errors.New("missing file ticket claims")
	}
	reqRel, err := metav1.NormalizeRelativeFilePath(reqPath, allowRoot)
	if err != nil {
		return err
	}
	claimPath := strings.TrimSpace(c.Path)
	if claimPath == "" || claimPath == "/" || claimPath == "." {
		return nil
	}
	claimRel, err := metav1.NormalizeRelativeFilePath(claimPath, true)
	if err != nil {
		return fmt.Errorf("invalid ticket path claim: %w", err)
	}
	if claimRel == "." || reqRel == claimRel || strings.HasPrefix(reqRel, claimRel+"/") {
		return nil
	}
	return fmt.Errorf("requested path %q is outside signed ticket path %q", reqPath, c.Path)
}

// AllowsTargetPath verifies that a rename targetPath is valid and matches any signed targetPath claim.
func (c *FileTicketClaims) AllowsTargetPath(targetPath string) error {
	if c == nil {
		return errors.New("missing file ticket claims")
	}
	targetRel, err := metav1.NormalizeRelativeFilePath(targetPath, false)
	if err != nil {
		return err
	}
	claimTarget := strings.TrimSpace(c.TargetPath)
	if claimTarget == "" {
		return nil
	}
	claimTargetRel, err := metav1.NormalizeRelativeFilePath(claimTarget, false)
	if err != nil {
		return fmt.Errorf("invalid ticket targetPath claim: %w", err)
	}
	if targetRel != claimTargetRel {
		return fmt.Errorf("requested targetPath %q does not match signed ticket targetPath %q", targetPath, c.TargetPath)
	}
	return nil
}

// AllowsPresignedURL verifies that backup/restore requests use a dedicated backup/restore ticket
// bound to the exact control-plane presigned URL.
func (c *FileTicketClaims) AllowsPresignedURL(action, rawURL string) error {
	if c == nil {
		return errors.New("backup and restore require a dedicated signed ticket")
	}
	if err := c.AllowsMethodAndAction(http.MethodPost, action); err != nil {
		return err
	}
	if strings.TrimSpace(c.PresignedURL) == "" {
		return errors.New("signed ticket is missing bound presignedURL")
	}
	if subtle.ConstantTimeCompare([]byte(c.PresignedURL), []byte(strings.TrimSpace(rawURL))) != 1 {
		return errors.New("presignedURL does not match signed ticket URL")
	}
	return nil
}

// ResolveQuotas enforces that request-supplied MaxBytes and RateLimit do not exceed the signed ticket quotas.
func (c *FileTicketClaims) ResolveQuotas(reqMaxBytes, reqRateLimit int64) (int64, int64, error) {
	if c == nil {
		return reqMaxBytes, reqRateLimit, nil
	}
	if reqMaxBytes < 0 || reqRateLimit < 0 {
		return 0, 0, errors.New("file transfer budget and rate limit cannot be negative")
	}
	maxBytes := reqMaxBytes
	if c.MaxBytes > 0 && (maxBytes <= 0 || maxBytes > c.MaxBytes) {
		maxBytes = c.MaxBytes
	}
	rateLimit := reqRateLimit
	if c.RateLimit > 0 && (rateLimit <= 0 || rateLimit > c.RateLimit) {
		rateLimit = c.RateLimit
	}
	return maxBytes, rateLimit, nil
}

// AllowsRestoreMapping enforces that restore mapping options match the signed ticket.
func (c *FileTicketClaims) AllowsRestoreMapping(sourcePath, targetPath string, replaceTarget bool) error {
	if c == nil {
		return nil
	}
	if err := matchSignedRelPath("sourcePath", sourcePath, c.SourcePath); err != nil {
		return err
	}
	if err := matchSignedRelPath("targetPath", targetPath, c.TargetPath); err != nil {
		return err
	}
	if c.ReplaceTarget != replaceTarget {
		return errors.New("restore replaceTarget does not match signed ticket")
	}
	return nil
}

func matchSignedRelPath(field, reqPath, claimPath string) error {
	reqRel, err := metav1.NormalizeArchiveEntryPath(reqPath, true)
	if err != nil {
		return fmt.Errorf("invalid restore %s: %w", field, err)
	}
	claimRel, err := metav1.NormalizeArchiveEntryPath(claimPath, true)
	if err != nil {
		return fmt.Errorf("invalid ticket %s: %w", field, err)
	}
	if subtle.ConstantTimeCompare([]byte(claimRel), []byte(reqRel)) != 1 {
		return fmt.Errorf("restore %s %q does not match signed ticket %q", field, reqRel, claimRel)
	}
	return nil
}
