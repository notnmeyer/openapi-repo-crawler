package main

import (
	"bufio"
	"context"
	"fmt"
	"log"
	"os"
	"regexp"

	git "github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing"
	"github.com/go-git/go-git/v5/plumbing/object"
	"github.com/go-git/go-git/v5/plumbing/transport/http"
	"github.com/google/go-github/github"
	"github.com/joho/godotenv"
	"golang.org/x/oauth2"
)

func init() {
	err := godotenv.Load()
	if err != nil {
		log.Fatal("Error loading .env file")
	}
}

func main() {
	repos := getReposList()

	for _, repo := range repos {
		// fmt.Println(*repo.FullName)
		cloneDir, err := cloneRepo(&repo)
		if err != nil {
			fmt.Println(err)
			continue
		}
		findOpenAPIFiles(cloneDir, repo)
	}
}

// get all non-archived repos for an org
func getReposList() []github.Repository {
	client := newGitHubClient()
	repos, _, err := client.Repositories.ListByOrg(context.Background(), os.Getenv("GH_ORG"), &github.RepositoryListByOrgOptions{
		ListOptions: github.ListOptions{PerPage: 1000},
		Type:        "all",
	})
	if err != nil {
		log.Fatal(err)
	}

	var reposList []github.Repository
	for _, repo := range repos {
		if !repo.GetArchived() {
			reposList = append(reposList, *repo)
		}
	}

	return reposList
}

func newGitHubClient() *github.Client {
	ctx := context.Background()
	ts := oauth2.StaticTokenSource(
		&oauth2.Token{AccessToken: os.Getenv("GH_PAT")},
	)
	tc := oauth2.NewClient(ctx, ts)
	return github.NewClient(tc)
}

func cloneRepo(repo *github.Repository) (directory string, err error) {
	fmt.Printf("Cloning %s...\n", *repo.FullName)
	directory = "/tmp/crawler/" + *repo.FullName
	_, err = git.PlainClone(directory, false, &git.CloneOptions{
		Auth: &http.BasicAuth{
			Username: "abc123", // yes, this can be anything except an empty string
			Password: os.Getenv("GH_PAT"),
		},
		URL: repo.GetCloneURL(),
		// Progress: os.Stdout,
		Progress: nil,
		Depth:    1,
	})
	if err != nil {
		return "", err
	}
	return directory, nil
}

func findOpenAPIFiles(directory string, gitHubRepo github.Repository) {
	repo, err := git.PlainOpen(directory)
	if err != nil {
		log.Fatal(err)
	}

	h, err := repo.ResolveRevision(plumbing.Revision("HEAD"))
	if err != nil {
		log.Fatal(err)
	}

	commit, err := repo.CommitObject(plumbing.NewHash(h.String()))
	if err != nil {
		log.Fatal(err)
	}

	tree, err := commit.Tree()
	if err != nil {
		log.Fatal(err)
	}

	// var openAPIFiles []string
	tree.Files().ForEach(func(f *object.File) error {
		if match, _ := regexp.MatchString(".(yml|yaml)", f.Name); match {
			// spew.Dump(f.Name)
			of, err := os.Open(directory + "/" + f.Name)
			if err != nil {
				log.Fatal(err)
			}

			scanner := bufio.NewScanner(of)
			for scanner.Scan() {
				if match, _ := regexp.MatchString("^openapi: \"\\d.\\d.\\d\"", scanner.Text()); match {
					fmt.Printf("\tOpenAPI spec found! %s\n", f.Name)
				}
			}
		}
		return nil
	})

	err = os.RemoveAll(directory)
	if err != nil {
		log.Fatal(err)
	}
}
