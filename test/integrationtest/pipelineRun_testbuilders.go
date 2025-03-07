package integrationtest

import (
	"time"

	api "github.com/SAP/stewardci-core/pkg/apis/steward/v1alpha1"
	builder "github.com/SAP/stewardci-core/test/builder"
	f "github.com/SAP/stewardci-core/test/framework"
	"github.com/SAP/stewardci-core/test/shared"
	v1 "k8s.io/api/core/v1"
)

// AllTestBuilders is a list of all test builders
var AllTestBuilders = []f.PipelineRunTestBuilder{
	PipelineRunAbort,
	PipelineRunSleep,
	PipelineRunFail,
	PipelineRunOK,
	PipelineRunK8SPlugin,
	PipelineRunWithSecret,
	PipelineRunWithSecretRename,
	PipelineRunWithSecretInvalidRename,
	PipelineRunWithSecretRenameDuplicate,
	PipelineRunWrongJenkinsfileRepo,
	PipelineRunWrongJenkinsfilePath,
	PipelineRunWrongJenkinsfileRepoWithUser,
}

// PipelineRunAbort is a PipelineRunTestBuilder to build a PipelineRunTest with aborted pipeline
func PipelineRunAbort(Namespace string, runID *api.CustomJSON) f.PipelineRunTest {
	return f.PipelineRunTest{
		PipelineRun: builder.PipelineRun("abort-", Namespace,
			builder.PipelineRunSpec(
				builder.LoggingWithRunID(runID),
				builder.Abort(),
				builder.JenkinsFileSpec(shared.ExamplePipelineRepoURL,
					"sleep/Jenkinsfile", shared.ExamplePipelineRepoRevision),
			)),
		Check:   f.PipelineRunHasStateResult(api.ResultAborted),
		Timeout: 15 * time.Second,
	}

}

// PipelineRunSleep is a PipelineRunTestBuilder to build PipelineRunTest which sleeps for one second
func PipelineRunSleep(Namespace string, runID *api.CustomJSON) f.PipelineRunTest {
	return f.PipelineRunTest{
		PipelineRun: builder.PipelineRun("sleep-", Namespace,
			builder.PipelineRunSpec(
				builder.LoggingWithRunID(runID),
				builder.JenkinsFileSpec(shared.ExamplePipelineRepoURL,
					"sleep/Jenkinsfile", shared.ExamplePipelineRepoRevision),
				builder.ArgSpec("SLEEP_FOR_SECONDS", "1"),
			)),
		Check:   f.PipelineRunHasStateResult(api.ResultSuccess),
		Timeout: 600 * time.Second,
	}
}

// PipelineRunFail is a PipelineRunTestBuilder to build PipelineRunTest which fails
func PipelineRunFail(Namespace string, runID *api.CustomJSON) f.PipelineRunTest {
	return f.PipelineRunTest{
		PipelineRun: builder.PipelineRun("error-", Namespace,
			builder.PipelineRunSpec(
				builder.LoggingWithRunID(runID),
				builder.JenkinsFileSpec(shared.ExamplePipelineRepoURL,
					"error/Jenkinsfile", shared.ExamplePipelineRepoRevision),
			)),
		Check:   f.PipelineRunHasStateResult(api.ResultErrorContent),
		Timeout: 600 * time.Second,
	}
}

// PipelineRunOK is a PipelineRunTestBuilder to build PipelineRunTest which succeeds
func PipelineRunOK(Namespace string, runID *api.CustomJSON) f.PipelineRunTest {
	return f.PipelineRunTest{
		PipelineRun: builder.PipelineRun("ok-", Namespace,
			builder.PipelineRunSpec(
				builder.LoggingWithRunID(runID),
				builder.JenkinsFileSpec(shared.ExamplePipelineRepoURL,
					"success/Jenkinsfile", shared.ExamplePipelineRepoRevision),

				builder.RunDetails("myJobName1", "myCause1", 17),
			)),
		Check:   f.PipelineRunHasStateResult(api.ResultSuccess),
		Timeout: 600 * time.Second,
	}
}

// PipelineRunK8SPlugin is a PipelineRunTestBuilder to build PipelineRunTest which uses k8s plugin
func PipelineRunK8SPlugin(Namespace string, runID *api.CustomJSON) f.PipelineRunTest {
	return f.PipelineRunTest{
		PipelineRun: builder.PipelineRun("k8s-", Namespace,
			builder.PipelineRunSpec(
				builder.LoggingWithRunID(runID),
				builder.JenkinsFileSpec(shared.ExamplePipelineRepoURL,
					"k8sPlugin/Jenkinsfile", shared.ExamplePipelineRepoRevision),

				builder.RunDetails("myK8SJob1", "myCause1", 18),
			)),
		Check:   f.PipelineRunHasStateResult(api.ResultSuccess),
		Timeout: 600 * time.Second,
	}
}

// PipelineRunWithSecret is a PipelineRunTestBuilder to build PipelineRunTest which uses Secrets
func PipelineRunWithSecret(Namespace string, runID *api.CustomJSON) f.PipelineRunTest {
	return f.PipelineRunTest{
		PipelineRun: builder.PipelineRun("with-secret-", Namespace,
			builder.PipelineRunSpec(
				builder.LoggingWithRunID(runID),
				builder.JenkinsFileSpec(shared.ExamplePipelineRepoURL,
					"secret/Jenkinsfile", shared.ExamplePipelineRepoRevision),
				builder.ArgSpec("SECRETID", "with-secret-foo"),
				builder.ArgSpec("EXPECTEDUSER", "bar"),
				builder.ArgSpec("EXPECTEDPWD", "baz"),
				builder.Secret("with-secret-foo"),
			)),
		Check:   f.PipelineRunHasStateResult(api.ResultSuccess),
		Timeout: 120 * time.Second,
		Secrets: []*v1.Secret{builder.SecretBasicAuth("with-secret-foo", Namespace, "bar", "baz")},
	}
}

// PipelineRunWithSecretRename is a PipelineRunTestBuilder to build PipelineRunTest which uses Secrets with rename annotation
func PipelineRunWithSecretRename(Namespace string, runID *api.CustomJSON) f.PipelineRunTest {
	return f.PipelineRunTest{
		PipelineRun: builder.PipelineRun("with-secret-rename-", Namespace,
			builder.PipelineRunSpec(
				builder.LoggingWithRunID(runID),
				builder.JenkinsFileSpec(shared.ExamplePipelineRepoURL,
					"secret/Jenkinsfile", shared.ExamplePipelineRepoRevision),
				builder.ArgSpec("SECRETID", "renamed-secret-new-name"),
				builder.ArgSpec("EXPECTEDUSER", "bar"),
				builder.ArgSpec("EXPECTEDPWD", "baz"),
				builder.Secret("renamed-secret-foo"),
			)),
		Check:   f.PipelineRunHasStateResult(api.ResultSuccess),
		Timeout: 120 * time.Second,
		Secrets: []*v1.Secret{builder.SecretBasicAuth("renamed-secret-foo", Namespace, "bar", "baz",
			builder.SecretRename("renamed-secret-new-name"))},
	}
}

// PipelineRunWithSecretInvalidRename is a PipelineRunTestBuilder to build PipelineRunTest which uses Secrets with an invalid rename annotation
func PipelineRunWithSecretInvalidRename(Namespace string, runID *api.CustomJSON) f.PipelineRunTest {
	return f.PipelineRunTest{
		PipelineRun: builder.PipelineRun("with-secret-invalid-rename-", Namespace,
			builder.PipelineRunSpec(
				builder.LoggingWithRunID(runID),
				builder.JenkinsFileSpec(shared.ExamplePipelineRepoURL,
					"secret/Jenkinsfile", shared.ExamplePipelineRepoRevision),
				builder.ArgSpec("SECRETID", "InvalidName"),
				builder.ArgSpec("EXPECTEDUSER", "bar"),
				builder.ArgSpec("EXPECTEDPWD", "baz"),
				builder.Secret("invalid-secret-foo"),
			)),
		Check:   f.PipelineRunHasStateResult(api.ResultErrorContent),
		Timeout: 120 * time.Second,
		Secrets: []*v1.Secret{builder.SecretBasicAuth("invalid-secret-foo", Namespace, "bar", "baz",
			builder.SecretRename("InvalidName"))},
	}
}

// PipelineRunWithSecretRenameDuplicate is a PipelineRunTestBuilder to build PipelineRunTest which uses Secrets with an invalid rename annotation
func PipelineRunWithSecretRenameDuplicate(Namespace string, runID *api.CustomJSON) f.PipelineRunTest {
	return f.PipelineRunTest{
		PipelineRun: builder.PipelineRun("with-secret-duplicate-", Namespace,
			builder.PipelineRunSpec(
				builder.LoggingWithRunID(runID),
				builder.JenkinsFileSpec(shared.ExamplePipelineRepoURL,
					"secret/Jenkinsfile", shared.ExamplePipelineRepoRevision),
				builder.ArgSpec("SECRETID", "duplicate"),
				builder.ArgSpec("EXPECTEDUSER", "bar"),
				builder.ArgSpec("EXPECTEDPWD", "baz"),
				builder.Secret("duplicate-secret-foo"),
				builder.Secret("duplicate-secret-bar"),
			)),
		Check:   f.PipelineRunHasStateResult(api.ResultErrorContent),
		Timeout: 120 * time.Second,
		Secrets: []*v1.Secret{
			builder.SecretBasicAuth("duplicate-secret-foo", Namespace, "bar", "baz",
				builder.SecretRename("duplicate")),
			builder.SecretBasicAuth("duplicate-secret-bar", Namespace, "bar", "baz",
				builder.SecretRename("duplicate"))},
	}
}

// PipelineRunMissingSecret is a PipelineRunTestBuilder to build PipelineRunTest which uses Secrets
func PipelineRunMissingSecret(Namespace string, runID *api.CustomJSON) f.PipelineRunTest {
	return f.PipelineRunTest{
		PipelineRun: builder.PipelineRun("missing-secret-", Namespace,
			builder.PipelineRunSpec(
				builder.LoggingWithRunID(runID),
				builder.JenkinsFileSpec(shared.ExamplePipelineRepoURL,
					"secret/Jenkinsfile", shared.ExamplePipelineRepoRevision),
				builder.ArgSpec("SECRETID", "foo"),
				builder.ArgSpec("EXPECTEDUSER", "bar"),
				builder.ArgSpec("EXPECTEDPWD", "baz"),
			)),
		Check:   f.PipelineRunHasStateResult(api.ResultErrorContent),
		Timeout: 120 * time.Second,
		Secrets: []*v1.Secret{builder.SecretBasicAuth("missing-secret-foo", Namespace, "bar", "baz")},
	}
}

// PipelineRunWrongJenkinsfileRepo is a PipelineRunTestBuilder to build PipelineRunTest with wrong jenkinsfile repo url
func PipelineRunWrongJenkinsfileRepo(Namespace string, runID *api.CustomJSON) f.PipelineRunTest {
	return f.PipelineRunTest{
		PipelineRun: builder.PipelineRun("wrong-jenkinsfile-repo-", Namespace,
			builder.PipelineRunSpec(
				builder.LoggingWithRunID(runID),
				builder.JenkinsFileSpec("https://github.com/SAP/steward-foo",
					"Jenkinsfile", shared.ExamplePipelineRepoRevision),
			)),
		Check:   f.PipelineRunHasStateResult(api.ResultErrorContent),
		Timeout: 300 * time.Second,
	}
}

// PipelineRunWrongJenkinsfileRepoWithUser is a PipelineRunTestBuilder to build PipelineRunTest with wrong jenkinsfile repo url
func PipelineRunWrongJenkinsfileRepoWithUser(Namespace string, runID *api.CustomJSON) f.PipelineRunTest {
	return f.PipelineRunTest{
		PipelineRun: builder.PipelineRun("wrong-jenkinsfile-repo-user-", Namespace,
			builder.PipelineRunSpec(
				builder.LoggingWithRunID(runID),
				builder.JenkinsFileSpec("https://github.com/SAP/steward-foo",
					"Jenkinsfile", shared.ExamplePipelineRepoRevision,
					builder.RepoAuthSecret("repo-auth"),
				),
			)),
		Secrets: []*v1.Secret{builder.SecretBasicAuth("repo-auth", Namespace, "bar", "baz")},
		Check:   f.PipelineRunHasStateResult(api.ResultErrorContent),
		Timeout: 300 * time.Second,
	}
}

// PipelineRunWrongJenkinsfilePath is a PipelineRunTestBuilder to build PipelineRunTest with wrong jenkinsfile path
func PipelineRunWrongJenkinsfilePath(Namespace string, runID *api.CustomJSON) f.PipelineRunTest {
	return f.PipelineRunTest{
		PipelineRun: builder.PipelineRun("wrong-jenkinsfile-path-", Namespace,
			builder.PipelineRunSpec(
				builder.LoggingWithRunID(runID),
				builder.JenkinsFileSpec(shared.ExamplePipelineRepoURL,
					"not_existing_path/Jenkinsfile", shared.ExamplePipelineRepoRevision),
			)),
		Check: f.PipelineRunMessageOnFinished(`Command ['/app/bin/jenkinsfile-runner' '-w' '/app/jenkins' '-p' '/usr/share/jenkins/ref/plugins' '--runHome' '/jenkins_home' '--no-sandbox' '--build-number' '1' '-f' 'not_existing_path/Jenkinsfile'] failed with exit code 255
Error output:
no Jenkinsfile in current directory.`),
		Timeout: 120 * time.Second,
	}
}
