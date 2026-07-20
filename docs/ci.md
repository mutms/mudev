# CI usage (forgejo-runner)

mudev assembles a Moodle test-site **code tree** from a recipe. The CI job is responsible for
everything after that: database, `config.php`, and Moodle's phpunit/behat init scripts.

## Principles

- **Self-contained.** Ship a pinned static `mudev` binary in the runner image and check the
  plugin/recipe YAML into the CI workspace — no dependency on `/opt/mdl-*`.
- **Non-interactive.** Every command is env/flag driven; no prompts; meaningful exit codes.
- **Use a recipe whose URLs suit CI.** mudev has no authentication settings — it hands each URL
  to git untouched — so a CI recipe names `https://…` remotes and the public MuTMS repositories
  clone with no keys at all.

## Environment

```sh
export MUDEV_PLUGINS_DIRECTORY="$CI_WORKSPACE/mdl-plugins"
export MUDEV_RECIPES_DIRECTORY="$CI_WORKSPACE/mdl-recipes"
```

## Private repositories

Credentials are git's business, configured once in the runner rather than passed through mudev.
Both of the usual approaches work, and both cover every repository the job touches:

```sh
# a deploy key / SSH agent, for git@… remotes
ssh-add "$RUNNER_SSH_KEY"

# or a token for https remotes, scoped to one host
git config --global url."https://$CI_TOKEN@forge.example.org/".insteadOf \
                       "https://forge.example.org/"
```

Prefer the `insteadOf` form over putting a token in the recipe: the recipe is checked into a
repository, and the git config is not.

## Example job

```sh
mkdir -p "$CI_WORKSPACE/moodle" && cd "$CI_WORKSPACE/moodle"   # prepare + own the dir first
mudev clone mutms/full/5.2.1.01                                # identifier → mdl-recipes; assembles into cwd
# (or: mudev clone ./recipe.yaml   to clone a recipe file directly)
# → CI script then sets up DB + config.php and runs phpunit/behat init
```

`clone` exits non-zero on any failure and names the checkout that failed, so the job stops
where the problem is. It is also idempotent, which makes a cached workspace cheap: re-running
it against a warm directory brings the tree up to the recipe instead of rebuilding it.

Use a **pinned edition** recipe (`mutms/full/5.2.1.01`), not a `dev/` one: an edition pins every
plugin to a tag or commit, so the same job produces the same tree next month. Clones are
currently full, not shallow — see [ROADMAP.md](../ROADMAP.md).
