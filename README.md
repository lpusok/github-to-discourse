#github-to-discourse

## Dry run

Print the issues and actions without actually modifying any resources.

`go run . --mode=dry --repo-src=steplib https://bitrise-steplib-collection.s3.amazonaws.com/spec.json`

## Cherry pick repos

Provide specific repos to process.

`go run . --mode=dry --repo-src=cherry https://github.com/lszucs/github-sandbox,https://github.com/bitrise-core/bitrise-init`

## Live run

If confident, switch to `live` mode.

`go run . --mode=live --repo-src=cherry https://github.com/lszucs/github-sandbox`


