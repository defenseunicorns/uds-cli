#!/usr/bin/env sh

# This script creates and/or updates a nightly tag for the uds-cli repo
# The nightly tag is created from the latest commit on the main branch

tag="0.0.0-nightly"

# get oid and repositoryId for GH API Graphql Mutation
oid=$(gh api graphql -f query='
              {
                repository(owner: "defenseunicorns", name: "uds-cli") {
                  id
                  ref(qualifiedName: "refs/heads/main") {
                    target {
                      ... on Commit {
                        oid
                      }
                    }
                  }
                }
              }' | jq -r '.data.repository.ref.target.oid')

repositoryId=$(gh api graphql -f query='
              {
                repository(owner: "defenseunicorns", name: "uds-cli") {
                  id
                }
              }' | jq -r '.data.repository.id')


# get existing tag and save to existingTag var
existingRefId=$(gh api graphql -f query='
                {
                  repository(owner: "defenseunicorns", name: "uds-cli") {
                    ref(qualifiedName: "refs/tags/'$tag'") {
                      id
                    }
                  }
                }' | jq -r '.data.repository.ref.id')

# remove any existing  nightly tags
gh api graphql -f query='
mutation DeleteRef {
  deleteRef(input:{refId:"'$existingRefId'"}) {
    clientMutationId
  }
}' --silent &&

echo "Existing '$tag' tag removed successfully"

# create a signed nightly tag
gh api graphql -f query='
mutation CreateRef {
  createRef(input:{name:"refs/tags/'$tag'",oid:"'$oid'",repositoryId:"'$repositoryId'"}) {
        ref {
          id
          name
        }
    }
}' --silent &&

echo "New '$tag' tag created successfully"
