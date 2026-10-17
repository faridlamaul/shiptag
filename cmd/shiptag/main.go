package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"strings"

	"github.com/faridlamaul/shiptag/pkg/changelog"
	"github.com/faridlamaul/shiptag/pkg/githubclient"
	"github.com/faridlamaul/shiptag/pkg/semver"
	"github.com/spf13/cobra"
)

var (
	flagRepo       string
	flagToken      string
	flagRef        string
	flagBump       string
	flagVersion    string
	flagDryRun     bool
	flagDraft      bool
	flagPrerelease bool
)

func main() {
	log.SetFlags(log.LstdFlags | log.Lshortfile)

	rootCmd := &cobra.Command{
		Use:   "shiptag",
		Short: "Semantic versioning and release automation for GitHub",
		Long: `⚡ shiptag — Semantic Versioning and Release Automation for GitHub

Analyzes Conventional Commits, calculates the next semantic version tag,
generates changelog release notes, and publishes GitHub Releases.`,
	}

	rootCmd.PersistentFlags().StringVarP(&flagRepo, "repo", "r", os.Getenv("GITHUB_REPOSITORY"), "GitHub repository slug (owner/repo)")
	rootCmd.PersistentFlags().StringVarP(&flagToken, "token", "t", "", "GitHub token (or GITHUB_TOKEN env)")
	rootCmd.PersistentFlags().StringVar(&flagRef, "ref", getEnvDefault("GITHUB_SHA", "main"), "Target commit SHA or branch name")

	// 1. tag command: semantic bump + tag + release notes
	tagCmd := &cobra.Command{
		Use:   "tag",
		Short: "Calculate next version, create git tag, and publish GitHub Release",
		RunE:  runTag,
	}
	tagCmd.Flags().StringVarP(&flagBump, "bump", "b", "auto", "Version bump: auto, major, minor, or patch")
	tagCmd.Flags().StringVarP(&flagVersion, "version", "v", "", "Explicit version override (e.g. v1.2.3)")
	tagCmd.Flags().BoolVar(&flagDryRun, "dry-run", false, "Preview next version and release notes without making changes")
	tagCmd.Flags().BoolVar(&flagDraft, "draft", false, "Create release as draft")
	tagCmd.Flags().BoolVar(&flagPrerelease, "prerelease", false, "Create release as prerelease")

	// 2. changelog command: preview release notes
	changelogCmd := &cobra.Command{
		Use:   "changelog",
		Short: "Generate and print release notes to stdout",
		RunE:  runChangelog,
	}
	changelogCmd.Flags().StringVarP(&flagVersion, "version", "v", "", "Target version name")

	// 3. next-version command: print only next version string (ideal for CI pipelines)
	nextVerCmd := &cobra.Command{
		Use:   "next-version",
		Short: "Calculate and print next version string to stdout",
		RunE:  runNextVersion,
	}
	nextVerCmd.Flags().StringVarP(&flagBump, "bump", "b", "auto", "Version bump: auto, major, minor, or patch")

	rootCmd.AddCommand(tagCmd, changelogCmd, nextVerCmd)

	if err := rootCmd.Execute(); err != nil {
		os.Exit(1)
	}
}

func getEnvDefault(key, fallback string) string {
	if val := os.Getenv(key); val != "" {
		return val
	}
	return fallback
}

func runTag(cmd *cobra.Command, args []string) error {
	ctx := context.Background()
	client, err := githubclient.NewClient(ctx, flagToken, flagRepo)
	if err != nil {
		return err
	}

	tags, err := client.ListTags(ctx)
	if err != nil {
		return err
	}

	latestTag, _ := semver.LatestStrictTag(tags)
	log.Printf("Latest semantic tag: %q", latestTag)

	// Fetch commits between latest tag and target ref
	commits, err := client.GetCommitsSinceTag(ctx, latestTag, flagRef)
	if err != nil {
		return err
	}
	log.Printf("Found %d commit(s) since %q on %s", len(commits), latestTag, flagRef)

	if len(commits) == 0 && flagVersion == "" {
		log.Println("No new commits found. Skipping release.")
		return nil
	}

	// Determine next version
	nextVer := flagVersion
	hasBreaking := false
	hasFeatures := false

	// Generate temporary notes to detect breaking / features
	_, hasBreaking, hasFeatures = changelog.GenerateReleaseNotes(commits, changelog.ReleaseMeta{
		RepoOwner:   client.Owner,
		RepoName:    client.Repo,
		PreviousTag: latestTag,
		NewTag:      "preview",
	})

	if nextVer == "" {
		bumpType := flagBump
		if bumpType == "auto" {
			bumpType = semver.AutoBumpType(hasBreaking, hasFeatures)
		}
		log.Printf("Determined bump type: %s (breaking=%v, feat=%v)", bumpType, hasBreaking, hasFeatures)

		nextVer, err = semver.NextVersion(latestTag, bumpType)
		if err != nil {
			return err
		}
	}
	log.Printf("Target version: %s", nextVer)

	// Generate final release notes with target version
	notes, _, _ := changelog.GenerateReleaseNotes(commits, changelog.ReleaseMeta{
		RepoOwner:   client.Owner,
		RepoName:    client.Repo,
		PreviousTag: latestTag,
		NewTag:      nextVer,
		Branch:      flagRef,
	})

	if flagDryRun {
		fmt.Printf("\n[DRY-RUN] Target Version: %s\n", nextVer)
		fmt.Printf("[DRY-RUN] Target Commit:  %s\n", flagRef)
		fmt.Printf("\n--- RELEASE NOTES PREVIEW ---\n%s\n", notes)
		return nil
	}

	// 1. Create Git Tag
	log.Printf("Creating git tag %s on %s...", nextVer, flagRef)
	if err := client.CreateTag(ctx, nextVer, flagRef); err != nil {
		// If tag already exists or failed, log warning and try release
		if strings.Contains(err.Error(), "already exists") {
			log.Printf("Tag %s already exists, proceeding to update release...", nextVer)
		} else {
			return err
		}
	}

	// 2. Create GitHub Release
	log.Printf("Creating GitHub Release for %s...", nextVer)
	rel, err := client.CreateRelease(ctx, nextVer, flagRef, nextVer, notes, flagDraft, flagPrerelease)
	if err != nil {
		return err
	}

	log.Printf("✓ Successfully created release: %s", rel.GetHTMLURL())
	fmt.Printf("RELEASE_VERSION=%s\n", nextVer)
	fmt.Printf("RELEASE_URL=%s\n", rel.GetHTMLURL())

	return nil
}

func runChangelog(cmd *cobra.Command, args []string) error {
	ctx := context.Background()
	client, err := githubclient.NewClient(ctx, flagToken, flagRepo)
	if err != nil {
		return err
	}

	tags, err := client.ListTags(ctx)
	if err != nil {
		return err
	}

	latestTag, _ := semver.LatestStrictTag(tags)
	commits, err := client.GetCommitsSinceTag(ctx, latestTag, flagRef)
	if err != nil {
		return err
	}

	tagLabel := flagVersion
	if tagLabel == "" {
		tagLabel = "Next Version"
	}

	notes, _, _ := changelog.GenerateReleaseNotes(commits, changelog.ReleaseMeta{
		RepoOwner:   client.Owner,
		RepoName:    client.Repo,
		PreviousTag: latestTag,
		NewTag:      tagLabel,
		Branch:      flagRef,
	})

	fmt.Println(notes)
	return nil
}

func runNextVersion(cmd *cobra.Command, args []string) error {
	ctx := context.Background()
	client, err := githubclient.NewClient(ctx, flagToken, flagRepo)
	if err != nil {
		return err
	}

	tags, err := client.ListTags(ctx)
	if err != nil {
		return err
	}

	latestTag, _ := semver.LatestStrictTag(tags)
	commits, err := client.GetCommitsSinceTag(ctx, latestTag, flagRef)
	if err != nil {
		return err
	}

	_, hasBreaking, hasFeatures := changelog.GenerateReleaseNotes(commits, changelog.ReleaseMeta{
		RepoOwner:   client.Owner,
		RepoName:    client.Repo,
		PreviousTag: latestTag,
		NewTag:      "preview",
	})

	bumpType := flagBump
	if bumpType == "auto" {
		bumpType = semver.AutoBumpType(hasBreaking, hasFeatures)
	}

	nextVer, err := semver.NextVersion(latestTag, bumpType)
	if err != nil {
		return err
	}

	fmt.Println(nextVer)
	return nil
}
