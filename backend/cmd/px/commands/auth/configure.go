package auth

import (
	"bufio"
	"bytes"
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/json"
	"encoding/pem"
	"errors"
	"fmt"
	"io"
	"math/big"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/spf13/cobra"
	"golang.org/x/term"
)

const (
	defaultAdminAPI          = "http://localhost:8077/api"
	defaultCredentialsPath   = "~/.powerx/credentials.json"
	defaultMTLSDirectory     = "~/.powerx/cli"
	clientCertFilename       = "client.crt"
	clientKeyFilename        = "client.key"
	clientCAFilename         = "ca.crt"
	defaultClientValidDays   = 90
	generatedMetadataSubject = "PowerX Dev Client"
)

var (
	configureCmd = &cobra.Command{
		Use:   "configure",
		Short: "Configure CLI credentials and mTLS bundle",
		RunE:  runConfigure,
	}

	configureOpts = struct {
		api             string
		tenant          string
		identifier      string
		password        string
		credentialsFile string
		force           bool

		skipMTLS       bool
		generateMTLS   bool
		mtlsDir        string
		mtlsCertSource string
		mtlsKeySource  string
		mtlsCASource   string
		mtlsCommonName string
		mtlsValidDays  int
		mtlsOnly       bool
	}{
		api:             defaultAdminAPI,
		credentialsFile: defaultCredentialsPath,
		mtlsDir:         defaultMTLSDirectory,
		generateMTLS:    true,
		mtlsValidDays:   defaultClientValidDays,
	}
)

func init() {
	Command.AddCommand(configureCmd)

	configureCmd.Flags().StringVar(&configureOpts.api, "api", configureOpts.api, "PowerX Admin API base URL (e.g. http://localhost:8077/api)")
	configureCmd.Flags().StringVar(&configureOpts.tenant, "tenant", configureOpts.tenant, "Tenant identifier (optional)")
	configureCmd.Flags().StringVar(&configureOpts.identifier, "identifier", configureOpts.identifier, "Login identifier (email/phone/username)")
	configureCmd.Flags().StringVar(&configureOpts.password, "password", configureOpts.password, "Login password (leave empty to prompt)")
	configureCmd.Flags().StringVar(&configureOpts.credentialsFile, "credentials", configureOpts.credentialsFile, "Path to write credentials JSON")
	configureCmd.Flags().BoolVar(&configureOpts.force, "force", configureOpts.force, "Overwrite existing credentials and certificate files")

	configureCmd.Flags().BoolVar(&configureOpts.skipMTLS, "skip-mtls", configureOpts.skipMTLS, "Skip mTLS bundle setup")
	configureCmd.Flags().BoolVar(&configureOpts.generateMTLS, "mtls-generate", configureOpts.generateMTLS, "Generate a self-signed mTLS bundle when sources are not provided")
	configureCmd.Flags().StringVar(&configureOpts.mtlsDir, "mtls-dir", configureOpts.mtlsDir, "Directory to store the mTLS bundle")
	configureCmd.Flags().StringVar(&configureOpts.mtlsCertSource, "mtls-cert", configureOpts.mtlsCertSource, "Existing client certificate to copy")
	configureCmd.Flags().StringVar(&configureOpts.mtlsKeySource, "mtls-key", configureOpts.mtlsKeySource, "Existing client key to copy")
	configureCmd.Flags().StringVar(&configureOpts.mtlsCASource, "mtls-ca", configureOpts.mtlsCASource, "Existing CA certificate to copy")
	configureCmd.Flags().StringVar(&configureOpts.mtlsCommonName, "mtls-cn", configureOpts.mtlsCommonName, "Common Name for generated client certificate (defaults to identifier)")
	configureCmd.Flags().IntVar(&configureOpts.mtlsValidDays, "mtls-valid-days", configureOpts.mtlsValidDays, "Validity window for generated certificates (days)")
	configureCmd.Flags().BoolVar(&configureOpts.mtlsOnly, "mtls-only", configureOpts.mtlsOnly, "Skip Admin API login and only prepare the mTLS bundle")
}

type loginSuccess struct {
	TokenType    string `json:"token_type"`
	AccessToken  string `json:"access_token"`
	ExpiresIn    int    `json:"expires_in"`
	RefreshToken string `json:"refresh_token"`
	Scope        string `json:"scope"`
}

type apiSuccess[T any] struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
	Data    T      `json:"data"`
}

type credentialsFile struct {
	API        string        `json:"api"`
	Tenant     string        `json:"tenant,omitempty"`
	Identifier string        `json:"identifier"`
	TokenType  string        `json:"token_type"`
	Access     string        `json:"access_token"`
	Refresh    string        `json:"refresh_token,omitempty"`
	ExpiresAt  time.Time     `json:"expires_at"`
	UpdatedAt  time.Time     `json:"updated_at"`
	Scope      string        `json:"scope,omitempty"`
	MTLS       *mtlsMetadata `json:"mtls,omitempty"`
}

type mtlsMetadata struct {
	Directory  string    `json:"directory"`
	CertPath   string    `json:"cert_path"`
	KeyPath    string    `json:"key_path"`
	CAPath     string    `json:"ca_path"`
	Generated  bool      `json:"generated"`
	CommonName string    `json:"common_name,omitempty"`
	ValidUntil time.Time `json:"valid_until,omitempty"`
}

func runConfigure(cmd *cobra.Command, _ []string) error {
	ctx := cmd.Context()
	var err error
	apiBase := strings.TrimRight(strings.TrimSpace(configureOpts.api), "/")
	if apiBase == "" {
		return errors.New("api base URL is required")
	}

	identifier := strings.TrimSpace(configureOpts.identifier)
	if identifier == "" {
		val, err := promptInput(cmd, "Identifier (email/username): ", false)
		if err != nil {
			return err
		}
		identifier = val
	}
	if identifier == "" {
		return errors.New("identifier is required")
	}

	password := configureOpts.password
	if !configureOpts.mtlsOnly {
		if password == "" {
			val, err := promptInput(cmd, "Password: ", true)
			if err != nil {
				return err
			}
			password = val
		}
		if password == "" {
			return errors.New("password is required")
		}
	}

	if configureOpts.mtlsValidDays <= 0 {
		return errors.New("mtls-valid-days must be positive")
	}

	var loginData *loginSuccess
	if !configureOpts.mtlsOnly {
		var loginURL string
		loginData, loginURL, err = performLogin(ctx, apiBase, configureOpts.tenant, identifier, password)
		if err != nil {
			return fmt.Errorf("login via %s failed: %w", loginURL, err)
		}
	}

	credPath, err := expandPath(configureOpts.credentialsFile)
	if err != nil {
		return err
	}
	if credPath == "" {
		return errors.New("credentials path cannot be empty")
	}
	if !configureOpts.force {
		if _, err := os.Stat(credPath); err == nil {
			return fmt.Errorf("credentials file already exists at %s (use --force to overwrite)", credPath)
		}
	}
	if err := os.MkdirAll(filepath.Dir(credPath), 0o700); err != nil {
		return fmt.Errorf("create credentials parent dir: %w", err)
	}

	var mtlsInfo *mtlsMetadata
	if !configureOpts.skipMTLS {
		mtlsInfo, err = configureMTLSBundle(identifier)
		if err != nil {
			return err
		}
	}

	cred := credentialsFile{
		API:        apiBase,
		Tenant:     strings.TrimSpace(configureOpts.tenant),
		Identifier: identifier,
		MTLS:       mtlsInfo,
		UpdatedAt:  time.Now().UTC(),
	}
	if loginData != nil {
		cred.TokenType = loginData.TokenType
		cred.Access = loginData.AccessToken
		cred.Refresh = loginData.RefreshToken
		cred.Scope = loginData.Scope
		cred.ExpiresAt = time.Now().Add(time.Duration(loginData.ExpiresIn) * time.Second).UTC()
	}
	if err := writeJSONFile(credPath, cred, 0o600); err != nil {
		return err
	}

	fmt.Fprintf(cmd.OutOrStdout(), "Credentials saved to %s\n", credPath)
	if mtlsInfo != nil {
		fmt.Fprintf(cmd.OutOrStdout(), "mTLS bundle ready under %s\n", mtlsInfo.Directory)
	}
	if configureOpts.mtlsOnly {
		fmt.Fprintln(cmd.OutOrStdout(), "Login skipped (--mtls-only); credentials file contains only API metadata.")
	}
	return nil
}

func performLogin(ctx context.Context, apiBase, tenant, identifier, password string) (*loginSuccess, string, error) {
	payload := map[string]string{
		"identifier": identifier,
		"password":   password,
	}
	if strings.TrimSpace(tenant) != "" {
		payload["tenant"] = strings.TrimSpace(tenant)
	}

	endpoints := buildLoginEndpoints(apiBase)
	var lastErr error
	for _, endpoint := range endpoints {
		var resp apiSuccess[loginSuccess]
		if err := doJSONRequest(ctx, http.MethodPost, endpoint, payload, &resp); err != nil {
			lastErr = err
			continue
		}
		if resp.Data.AccessToken == "" {
			lastErr = errors.New("login response missing access token")
			continue
		}
		return &resp.Data, endpoint, nil
	}
	return nil, strings.Join(endpoints, ", "), lastErr
}

func buildLoginEndpoints(apiBase string) []string {
	const loginPath = "/admin/user/auth/login"
	base := strings.TrimRight(strings.TrimSpace(apiBase), "/")
	if base == "" {
		return []string{loginPath}
	}
	var endpoints []string
	seen := make(map[string]struct{})
	add := func(prefix string) {
		prefix = strings.TrimRight(prefix, "/")
		url := prefix + loginPath
		if prefix == "" {
			url = loginPath
		}
		if _, ok := seen[url]; ok {
			return
		}
		seen[url] = struct{}{}
		endpoints = append(endpoints, url)
	}

	add(base)
	trimmed := base
	for _, suffix := range []string{"/api/v1", "/api"} {
		if strings.HasSuffix(trimmed, suffix) {
			trimmed = strings.TrimSuffix(trimmed, suffix)
			add(trimmed)
		}
	}

	root := trimmed
	for _, suffix := range []string{"/api", "/api/v1"} {
		if !strings.HasSuffix(base, suffix) {
			add(joinBase(root, suffix))
		}
	}
	return endpoints
}

func joinBase(base, suffix string) string {
	base = strings.TrimRight(base, "/")
	if base == "" {
		return strings.TrimSuffix(suffix, "/")
	}
	suffix = strings.TrimLeft(suffix, "/")
	return base + "/" + suffix
}

func doJSONRequest(ctx context.Context, method, url string, payload any, dest any) error {
	var body io.Reader
	if payload != nil {
		buf, err := json.Marshal(payload)
		if err != nil {
			return fmt.Errorf("encode payload: %w", err)
		}
		body = bytes.NewReader(buf)
	}
	req, err := http.NewRequestWithContext(ctx, method, url, body)
	if err != nil {
		return err
	}
	if payload != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	client := &http.Client{Timeout: 30 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return err
	}
	if resp.StatusCode >= 400 {
		return fmt.Errorf("%s: %s", resp.Status, strings.TrimSpace(string(data)))
	}
	if dest == nil {
		return nil
	}
	if err := json.Unmarshal(data, dest); err != nil {
		return fmt.Errorf("decode response: %w", err)
	}
	return nil
}

func configureMTLSBundle(identifier string) (*mtlsMetadata, error) {
	dir, err := expandPath(configureOpts.mtlsDir)
	if err != nil {
		return nil, err
	}
	if dir == "" {
		return nil, errors.New("mtls directory cannot be empty")
	}
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return nil, fmt.Errorf("create mTLS directory: %w", err)
	}

	certDst := filepath.Join(dir, clientCertFilename)
	keyDst := filepath.Join(dir, clientKeyFilename)
	caDst := filepath.Join(dir, clientCAFilename)

	if configureOpts.mtlsCertSource != "" || configureOpts.mtlsKeySource != "" || configureOpts.mtlsCASource != "" {
		if configureOpts.mtlsCertSource == "" || configureOpts.mtlsKeySource == "" || configureOpts.mtlsCASource == "" {
			return nil, errors.New("--mtls-cert, --mtls-key and --mtls-ca must be provided together")
		}
		if err := copyFile(configureOpts.mtlsCertSource, certDst, 0o644, configureOpts.force); err != nil {
			return nil, err
		}
		if err := copyFile(configureOpts.mtlsKeySource, keyDst, 0o600, configureOpts.force); err != nil {
			return nil, err
		}
		if err := copyFile(configureOpts.mtlsCASource, caDst, 0o644, configureOpts.force); err != nil {
			return nil, err
		}
		return &mtlsMetadata{
			Directory:  dir,
			CertPath:   certDst,
			KeyPath:    keyDst,
			CAPath:     caDst,
			Generated:  false,
			CommonName: extractCertificateCN(certDst),
			ValidUntil: readCertificateExpiry(certDst),
		}, nil
	}

	if !configureOpts.generateMTLS {
		if filesExist(certDst, keyDst, caDst) {
			return &mtlsMetadata{
				Directory:  dir,
				CertPath:   certDst,
				KeyPath:    keyDst,
				CAPath:     caDst,
				Generated:  false,
				CommonName: extractCertificateCN(certDst),
				ValidUntil: readCertificateExpiry(certDst),
			}, nil
		}
		return nil, errors.New("mTLS bundle not found; provide --mtls-* sources or enable --mtls-generate")
	}

	cn := strings.TrimSpace(configureOpts.mtlsCommonName)
	if cn == "" {
		cn = identifier
	}
	if cn == "" {
		cn = generatedMetadataSubject
	}

	clientNotAfter, err := generateSelfSignedBundle(dir, cn, configureOpts.mtlsValidDays, configureOpts.force)
	if err != nil {
		return nil, err
	}
	return &mtlsMetadata{
		Directory:  dir,
		CertPath:   certDst,
		KeyPath:    keyDst,
		CAPath:     caDst,
		Generated:  true,
		CommonName: cn,
		ValidUntil: clientNotAfter,
	}, nil
}

func generateSelfSignedBundle(dir, commonName string, validDays int, force bool) (time.Time, error) {
	certPath := filepath.Join(dir, clientCertFilename)
	keyPath := filepath.Join(dir, clientKeyFilename)
	caPath := filepath.Join(dir, clientCAFilename)
	if !force && filesExist(certPath, keyPath, caPath) {
		return time.Time{}, fmt.Errorf("mTLS files already exist in %s (use --force to overwrite)", dir)
	}

	now := time.Now().Add(-1 * time.Hour)
	validUntil := now.Add(time.Duration(validDays) * 24 * time.Hour)
	if validDays <= 0 {
		return time.Time{}, errors.New("validDays must be positive")
	}

	caSerial, err := rand.Int(rand.Reader, big.NewInt(1<<62))
	if err != nil {
		return time.Time{}, err
	}
	caKey, err := rsa.GenerateKey(rand.Reader, 3072)
	if err != nil {
		return time.Time{}, err
	}
	caTemplate := &x509.Certificate{
		SerialNumber: caSerial,
		Subject: pkix.Name{
			CommonName:   "PowerX Dev CA",
			Organization: []string{"PowerX"},
		},
		NotBefore:             now,
		NotAfter:              now.Add(365 * 24 * time.Hour),
		KeyUsage:              x509.KeyUsageCertSign | x509.KeyUsageCRLSign,
		ExtKeyUsage:           []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth, x509.ExtKeyUsageClientAuth},
		BasicConstraintsValid: true,
		IsCA:                  true,
	}
	caDER, err := x509.CreateCertificate(rand.Reader, caTemplate, caTemplate, &caKey.PublicKey, caKey)
	if err != nil {
		return time.Time{}, err
	}

	clientSerial, err := rand.Int(rand.Reader, big.NewInt(1<<62))
	if err != nil {
		return time.Time{}, err
	}
	clientKey, err := rsa.GenerateKey(rand.Reader, 3072)
	if err != nil {
		return time.Time{}, err
	}
	clientTemplate := &x509.Certificate{
		SerialNumber: clientSerial,
		Subject: pkix.Name{
			CommonName:   commonName,
			Organization: []string{"PowerX"},
		},
		NotBefore:    now,
		NotAfter:     validUntil,
		KeyUsage:     x509.KeyUsageDigitalSignature | x509.KeyUsageKeyEncipherment,
		ExtKeyUsage:  []x509.ExtKeyUsage{x509.ExtKeyUsageClientAuth},
		SubjectKeyId: []byte{1, 2, 3, 4, 6},
	}
	clientDER, err := x509.CreateCertificate(rand.Reader, clientTemplate, caTemplate, &clientKey.PublicKey, caKey)
	if err != nil {
		return time.Time{}, err
	}

	if err := writePEMFile(caPath, "CERTIFICATE", caDER, 0o644); err != nil {
		return time.Time{}, err
	}
	if err := writePEMFile(certPath, "CERTIFICATE", clientDER, 0o644); err != nil {
		return time.Time{}, err
	}
	keyBytes, err := x509.MarshalPKCS8PrivateKey(clientKey)
	if err != nil {
		return time.Time{}, err
	}
	if err := writePEMFile(keyPath, "PRIVATE KEY", keyBytes, 0o600); err != nil {
		return time.Time{}, err
	}
	return validUntil.UTC(), nil
}

func writePEMFile(path, blockType string, der []byte, perm os.FileMode) error {
	block := &pem.Block{Type: blockType, Bytes: der}
	buf := pem.EncodeToMemory(block)
	return writeFileAtomic(path, buf, perm)
}

func writeJSONFile(path string, payload any, perm os.FileMode) error {
	data, err := json.MarshalIndent(payload, "", "  ")
	if err != nil {
		return err
	}
	return writeFileAtomic(path, data, perm)
}

func writeFileAtomic(path string, data []byte, perm os.FileMode) error {
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, data, perm); err != nil {
		return err
	}
	if err := os.Rename(tmp, path); err != nil {
		_ = os.Remove(path)
		if err2 := os.Rename(tmp, path); err2 != nil {
			return err
		}
	}
	return nil
}

func copyFile(src, dst string, perm os.FileMode, force bool) error {
	srcPath, err := expandPath(src)
	if err != nil {
		return err
	}
	if srcPath == "" {
		return fmt.Errorf("invalid source path for %s", dst)
	}
	if !force {
		if _, err := os.Stat(dst); err == nil {
			return fmt.Errorf("file already exists: %s (use --force to overwrite)", dst)
		}
	}
	data, err := os.ReadFile(srcPath)
	if err != nil {
		return err
	}
	return writeFileAtomic(dst, data, perm)
}

func promptInput(cmd *cobra.Command, label string, secret bool) (string, error) {
	in := cmd.InOrStdin()
	if secret {
		if file, ok := in.(*os.File); ok && term.IsTerminal(int(file.Fd())) {
			fmt.Fprint(cmd.OutOrStdout(), label)
			bytes, err := term.ReadPassword(int(file.Fd()))
			fmt.Fprintln(cmd.OutOrStdout())
			if err != nil {
				return "", err
			}
			return strings.TrimSpace(string(bytes)), nil
		}
	}
	fmt.Fprint(cmd.OutOrStdout(), label)
	reader := bufio.NewReader(in)
	text, err := reader.ReadString('\n')
	if err != nil && !errors.Is(err, io.EOF) {
		return "", err
	}
	return strings.TrimSpace(text), nil
}

func expandPath(path string) (string, error) {
	path = strings.TrimSpace(path)
	if path == "" {
		return "", nil
	}
	if strings.HasPrefix(path, "~") {
		home, err := os.UserHomeDir()
		if err != nil {
			return "", err
		}
		path = filepath.Join(home, strings.TrimPrefix(path, "~"))
	}
	return filepath.Abs(path)
}

func filesExist(paths ...string) bool {
	for _, p := range paths {
		if _, err := os.Stat(p); err != nil {
			return false
		}
	}
	return true
}

func readCertificateExpiry(path string) time.Time {
	content, err := os.ReadFile(path)
	if err != nil {
		return time.Time{}
	}
	block, _ := pem.Decode(content)
	if block == nil {
		return time.Time{}
	}
	cert, err := x509.ParseCertificate(block.Bytes)
	if err != nil {
		return time.Time{}
	}
	return cert.NotAfter.UTC()
}

func extractCertificateCN(path string) string {
	content, err := os.ReadFile(path)
	if err != nil {
		return ""
	}
	block, _ := pem.Decode(content)
	if block == nil {
		return ""
	}
	cert, err := x509.ParseCertificate(block.Bytes)
	if err != nil {
		return ""
	}
	return cert.Subject.CommonName
}
