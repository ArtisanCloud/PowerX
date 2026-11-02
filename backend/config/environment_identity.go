package config

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"github.com/ArtisanCloud/PowerX/pkg/instanceidentity"
)

func (c *Config) EnvironmentIdentityFile() string { return c.environmentIdentityFile }

// ReadEnvironmentIdentity is read-only. A fresh installation is initialized by
// the existing environment lifecycle before Redis clients or workers are built.
func (c *Config) ReadEnvironmentIdentity(configFile string) error {
	if configFile == "" {
		return nil
	}
	if c.Recovery.Mode != "" || c.Recovery.ProfileFile != "" {
		return c.readEnvironmentIdentity(configFile, os.LookupEnv)
	}
	lookup, err := configurationEnvironment(configFile)
	if err != nil {
		return err
	}
	return c.readEnvironmentIdentity(configFile, lookup)
}

func (c *Config) readEnvironmentIdentity(configFile string, lookup environmentLookup) error {
	if configFile == "" {
		return nil
	}
	if c.Recovery.ProfileFile != "" {
		raw, err := os.ReadFile(c.Recovery.ProfileFile)
		if err != nil {
			return fmt.Errorf("environment profile is unreadable")
		}
		var p struct {
			Initial struct {
				ID   string `json:"id"`
				Root string `json:"root"`
			} `json:"initial"`
		}
		if json.Unmarshal(raw, &p) != nil || !instanceidentity.Valid(p.Initial.ID) {
			return fmt.Errorf("environment profile identity is invalid")
		}
		// Prepared recovery environments already use their recorded job ID as
		// Deployment.ID. The source environment keeps its original Deployment.ID.
		c.EnvironmentID = p.Initial.ID
		if c.Recovery.Root != "" && c.Recovery.Root != p.Initial.Root {
			if !instanceidentity.Valid(c.Recovery.JobID) {
				return fmt.Errorf("recovery environment identity is invalid")
			}
			c.EnvironmentID = c.Recovery.JobID
		}
		return nil
	}
	file, err := filepath.Abs(configFile)
	if err != nil {
		return fmt.Errorf("environment configuration path is invalid")
	}
	root := c.SystemSnapshots.Directory
	if root == "" {
		links, _ := lookup("POWERX_LINKS_ROOT")
		root, _ = instanceidentity.DefaultStorage(links)
	}
	c.environmentIdentityFile = filepath.Join(instanceidentity.CurrentDirectory(root, file), "installation.json")
	id, err := instanceidentity.ReadCurrentID(c.environmentIdentityFile, file)
	if err != nil {
		return err
	}
	c.EnvironmentID = id
	return nil
}
