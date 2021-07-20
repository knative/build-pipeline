# Tekton Pipelines Official Release Cheat Sheet

These steps provide a no-frills guide to performing an official release
of Tekton Pipelines. To follow these steps you'll need a checkout of
the pipelines repo, a terminal window and a text editor.

1. [Setup a context to connect to the dogfooding cluster](#setup-dogfooding-context) if you haven't already.

1. `cd` to root of Pipelines git checkout.

1. Select the commit you would like to build the release from, most likely the
   most recent commit at https://github.com/tektoncd/pipeline/commits/main
   and note the commit's hash.

1. Create environment variables for bash scripts in later steps.

    ```bash
    TEKTON_VERSION=# Example: v0.21.0
    TEKTON_RELEASE_GIT_SHA=# SHA of the release to be released
    TEKTON_IMAGE_REGISTRY=gcr.io/tekton-releases # only change if you want to publish to a different registry
    ```

1. Confirm commit SHA matches what you want to release.

    ```bash
    git show $TEKTON_RELEASE_GIT_SHA
    ```

1. Create a workspace template file:

   ```bash
   cat <<EOF > workspace-template.yaml
   spec:
     accessModes:
     - ReadWriteOnce
     resources:
       requests:
         storage: 1Gi
   EOF
   ```

1. Execute the release pipeline.

    **If you are back-porting include this flag: `--param=releaseAsLatest="false"`**

    ```bash
    tkn --context dogfooding pipeline start pipeline-release \
      --serviceaccount=release-right-meow \
      --param=gitRevision="${TEKTON_RELEASE_GIT_SHA}" \
      --param=serviceAccountPath=release.json \
      --param=versionTag="${TEKTON_VERSION}" \
      --param=releaseBucket=gs://tekton-releases/pipeline \
      --workspace name=release-secret,secret=release-secret \
      --workspace name=workarea,volumeClaimTemplateFile=workspace-template.yaml
    ```

1. Watch logs of pipeline-release.

1. Once the pipeline is complete, check its results:

    ```bash
    tkn --context dogfooding pr describe <pipeline-run-name>

    (...)
    📝 Results

    NAME                    VALUE
    ∙ commit-sha            ff6d7abebde12460aecd061ab0f6fd21053ba8a7
    ∙ release-file           https://storage.googleapis.com/tekton-releases-nightly/pipeline/previous/v20210223-xyzxyz/release.yaml
    ∙ release-file-no-tag    https://storage.googleapis.com/tekton-releases-nightly/pipeline/previous/v20210223-xyzxyz/release.notag.yaml

    (...)
    ```

    The `commit-sha` should match `$TEKTON_RELEASE_GIT_SHA`.
    The two URLs can be opened in the browser or via `curl` to download the release manifests.

1. The YAMLs are now released! Anyone installing Tekton Pipelines will get the new version. Time to create a new GitHub release announcement:

    1. Choose a name for the new release! The usual pattern is "< cat breed > < famous robot >" e.g. "Ragdoll Norby". Browse [the releases page](https://github.com/tektoncd/pipeline/releases) or run this command to check which names have already been used:

    ```bash
    curl \
      -H "Accept: application/vnd.github.v3+json" \
      https://api.github.com/repos/tektoncd/pipeline/releases\?per_page=100 \
      | jq ".[].name"
    ```

    1. Create additional environment variables

    ```bash
    TEKTON_OLD_VERSION=# Example: v0.11.1
    TEKTON_RELEASE_NAME=# The release name you just chose, e.g.: "Ragdoll Norby"
    TEKTON_PACKAGE=tektoncd/pipeline
    ```

    1. Create a `PipelineResource` of type `git`

    ```shell
    cat <<EOF | kubectl --context dogfooding create -f -
    apiVersion: tekton.dev/v1alpha1
    kind: PipelineResource
    metadata:
      name: tekton-pipelines-$(echo $TEKTON_VERSION | tr '.' '-')
      namespace: default
    spec:
      type: git
      params:
        - name: url
          value: 'https://github.com/tektoncd/pipeline'
        - name: revision
          value: ${TEKTON_RELEASE_GIT_SHA}
    EOF
    ```

    1. Execute the Draft Release task.

    ```bash
    tkn --context dogfooding task start \
      -i source="tekton-pipelines-$(echo $TEKTON_VERSION | tr '.' '-')" \
      -i release-bucket=pipeline-tekton-bucket \
      -p package="${TEKTON_PACKAGE}" \
      -p release-tag="${TEKTON_VERSION}" \
      -p previous-release-tag="${TEKTON_OLD_VERSION}" \
      -p release-name="${TEKTON_RELEASE_NAME}" \
      create-draft-release
    ```

    1. Watch logs of create-draft-release

    1. On successful completion, a URL will be logged. Visit that URL and look through the release notes.
      1. Manually add upgrade and deprecation notices based on the generated release notes
      1. Double-check that the list of commits here matches your expectations
         for the release. You might need to remove incorrect commits or copy/paste commits
         from the release branch. Refer to previous releases to confirm the expected format.

    1. Un-check the "This is a pre-release" checkbox since you're making a legit for-reals release!

    1. Publish the GitHub release once all notes are correct and in order.

1. Edit `README.md` on `master` branch, add entry to docs table with latest release links.

1. Push & make PR for updated `README.md`

1. Test release that you just made against your own cluster (note `--context my-dev-cluster`):

    ```bash
    # Test latest
    kubectl --context my-dev-cluster apply --filename https://storage.googleapis.com/tekton-releases/pipeline/latest/release.yaml
    ```

    ```bash
    # Test backport
    kubectl --context my-dev-cluster apply --filename https://storage.googleapis.com/tekton-releases/pipeline/previous/v0.11.2/release.yaml
    ```

1. Announce the release in Slack channels #general, #announcements and #pipelines.

1. Update [the catalog repo](https://github.com/tektoncd/catalog) test infrastructure
to use the new release by updating the `RELEASE_YAML` link in [e2e-tests.sh](https://github.com/tektoncd/catalog/blob/master/test/e2e-tests.sh).

1. Update the docs on the website with the latest version released:

    ```bash
    # From the website repo root, clean and up to date
    # Update the config
    cd sync
    python3 -m venv .venv
    source .venv/bin/activate    
    pip3 install -r requirements.txt
    ./versions.py add -p pipeline [version]

    # Stage, commit and push the change
    git checkout -b pipeline_vXYZ_docs
    git add -u
    git commit -m "Pipeline docs vXYZ"
    git push [fork remote] pipeline_vXYZ_docs
    ```

Congratulations, you're done!

## Setup dogfooding context

1. Configure `kubectl` to connect to
   [the dogfooding cluster](https://github.com/tektoncd/plumbing/blob/main/docs/dogfooding.md):

    ```bash
    gcloud container clusters get-credentials dogfooding --zone us-central1-a --project tekton-releases
    ```

1. Give [the context](https://kubernetes.io/docs/tasks/access-application-cluster/configure-access-multiple-clusters/)
   a short memorable name such as `dogfooding`:

   ```bash
   kubectl config rename-context gke_tekton-releases_us-central1-a_dogfooding dogfooding
   ```

1. **Important: Switch `kubectl` back to your own cluster by default.**

    ```bash
    kubectl config use-context my-dev-cluster
    ```
