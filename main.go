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

type OpenAPIRepo struct {
	Name      string
	FilePaths []string
}

func init() {
	err := godotenv.Load()
	if err != nil {
		log.Fatal("Error loading .env file")
	}
	fmt.Printf("Searching for repos with OpenAPI specs in GH org '%s'\n", os.Getenv("GH_ORG"))
}

func main() {
	repos := getReposList()

	openAPIRepos := []OpenAPIRepo{}
	errors := []error{}
	fmt.Printf("Examining %d repos", len(repos))
	for _, repo := range repos {
		cloneDir, err := cloneRepo(&repo)
		if err != nil {
			errors = append(errors, fmt.Errorf("Error cloning %s: %s", *repo.FullName, err))
			continue
		}

		files, err := findOpenAPIFiles(cloneDir, repo)
		if err != nil {
			log.Fatal(err)
		}

		if len(files) > 0 {
			openAPIRepos = append(openAPIRepos, OpenAPIRepo{
				Name:      *repo.FullName,
				FilePaths: files,
			})
		}
	}
	fmt.Print("Done!\n\n")

	for _, repo := range openAPIRepos {
		fmt.Println(repo.Name)
		for _, path := range repo.FilePaths {
			fmt.Printf("- %s\n", path)
		}
	}

	fmt.Println("")
	for _, err := range errors {
		fmt.Println(err)
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
	fmt.Print(".")
	// fmt.Printf("Cloning %s...\n", *repo.FullName)
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

func findOpenAPIFiles(directory string, gitHubRepo github.Repository) ([]string, error) {
	repo, err := git.PlainOpen(directory)
	if err != nil {
		return nil, err
	}

	h, err := repo.ResolveRevision(plumbing.Revision("HEAD"))
	if err != nil {
		return nil, err
	}

	commit, err := repo.CommitObject(plumbing.NewHash(h.String()))
	if err != nil {
		return nil, err
	}

	tree, err := commit.Tree()
	if err != nil {
		return nil, err
	}

	var paths []string
	ymlRegex := regexp.MustCompile(".(yml|yaml)")
	openAPIRegex := regexp.MustCompile("^openapi: \"\\d.\\d.\\d\"")

	tree.Files().ForEach(func(f *object.File) error {
		if match := ymlRegex.MatchString(f.Name); match {
			repoPath := fmt.Sprintf("%s/%s", directory, f.Name)
			of, err := os.Open(repoPath)
			if err != nil {
				log.Fatal(err)
			}

			scanner := bufio.NewScanner(of)
			for scanner.Scan() {
				if match := openAPIRegex.MatchString(scanner.Text()); match {
					paths = append(paths, f.Name)
				}
			}
		}
		return nil
	})

	err = os.RemoveAll(directory)
	if err != nil {
		log.Fatal(err)
	}

	return paths, nil
}
