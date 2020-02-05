package main

import (
	"bufio"
	"bytes"
	"context"
	"fmt"
	"io"
	"io/ioutil"
	"net"
	"os"
	"strings"
	"time"

	"github.com/spf13/cobra"
	"sigs.k8s.io/controller-runtime/pkg/runtime/signals"

	"github.com/sirupsen/logrus"
)

var (
	defaultServices      = []string{"image-registry.openshift-image-registry.svc"}
	defaultClusterDomain = "cluster.local"
	defaultHostsFile     = "/etc/hosts"
	defaultInterval      = 1 * time.Minute
)

func NewUpdateHostsCommand() *cobra.Command {
	var updater HostsUpdater

	cmd := &cobra.Command{
		Use:   "update-hosts",
		Short: "Updates /etc/hosts with service names",
		Long:  "Resolves the specified service names and adds them to a hosts file (i.e. /etc/hosts).",
		Run: func(cmd *cobra.Command, args []string) {
			if err := updater.Run(); err != nil {
				logrus.Error(err)
				os.Exit(1)
			}
		},
	}

	cmd.Flags().StringVarP(&updater.HostsFile, "hosts-file", "f", defaultHostsFile, "hosts file location")
	cmd.Flags().StringArrayVarP(&updater.Services, "services", "s", defaultServices, "service names to resolve and add to hosts file")
	cmd.Flags().DurationVarP(&updater.Interval, "interval", "i", defaultInterval, "how often to sync")
	cmd.Flags().StringVarP(&updater.Nameserver, "nameserver", "n", "", "the IP of the cluster DNS server (required)")
	cmd.Flags().StringVarP(&updater.ClusterDomain, "cluster-domain", "d", defaultClusterDomain, "the cluster domain suffix (required)")

	if err := cmd.MarkFlagRequired("nameserver"); err != nil {
		panic(err)
	}

	return cmd
}

type resolver interface {
	LookupIPAddr(ctx context.Context, host string) ([]net.IPAddr, error)
}

type HostsUpdater struct {
	HostsFile     string
	Services      []string
	Interval      time.Duration
	Nameserver    string
	ClusterDomain string

	resolver resolver
}

const marker = "openshift-generated-node-resolver"

func (s *HostsUpdater) Run() error {
	s.resolver = &net.Resolver{
		PreferGo: true,
		Dial: func(ctx context.Context, network, address string) (net.Conn, error) {
			d := net.Dialer{}
			return d.DialContext(ctx, "udp", net.JoinHostPort(s.Nameserver, "53"))
		},
	}

	signal := signals.SetupSignalHandler()
	ticker := time.NewTicker(s.Interval)
	for {
		select {
		case <-signal:
			logrus.Info("received signal, shutting down")
			return nil
		case <-ticker.C:
			if err := s.update(); err != nil {
				return err
			}
		}
	}
}

func (s *HostsUpdater) update() error {
	// Use the current hosts file contents to generate new content
	// for writing.
	currentContent, err := ioutil.ReadFile(s.HostsFile)
	if err != nil {
		return err
	}
	newContent, err := s.generate(bytes.NewBuffer(currentContent))
	if err != nil {
		return err
	}

	// Only update the hosts file if there's a change.
	if string(currentContent) == newContent {
		return nil
	}

	// Write a temporary file with the updated contents.
	newHostsFile, err := ioutil.TempFile("", "hosts")
	if err != nil {
		return err
	}
	if _, err := newHostsFile.WriteString(newContent); err != nil {
		return err
	}
	if err := newHostsFile.Close(); err != nil {
		return err
	}

	// Atomically replace the current hosts file with the temporary file.
	if err := os.Rename(newHostsFile.Name(), s.HostsFile); err != nil {
		return err
	}

	return nil
}

func (s *HostsUpdater) generate(hostsFile io.Reader) (string, error) {
	// Read the existing hosts file contents, excluding previously generated
	// lines, so we can re-generate the entries.
	var newContent string
	scanner := bufio.NewScanner(hostsFile)
	for scanner.Scan() {
		if !strings.Contains(scanner.Text(), marker) {
			newContent += scanner.Text() + "\n"
		}
	}
	if err := scanner.Err(); err != nil {
		return "", err
	}

	// Append latest entries to the new hosts file contents.
	var entries []string
	for _, svc := range s.Services {
		fqdn := strings.Join([]string{svc, s.ClusterDomain}, ".")
		ctx, _ := context.WithTimeout(context.Background(), 5*time.Second)
		ips, err := s.resolver.LookupIPAddr(ctx, fqdn)
		if err != nil {
			return "", fmt.Errorf("failed to look up IP for %s: %v", fqdn, err)
		}
		for _, ip := range ips {
			entries = append(entries, fmt.Sprintf("%s %s %s # %s", ip.String(), svc, fqdn, marker))
		}
	}
	for _, entry := range entries {
		newContent += entry + "\n"
	}
	return newContent, nil
}
