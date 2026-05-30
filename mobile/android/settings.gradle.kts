pluginManagement {
    repositories {
        google()
        mavenCentral()
        gradlePluginPortal()
    }
}

dependencyResolutionManagement {
    repositoriesMode.set(RepositoriesMode.FAIL_ON_PROJECT_REPOS)
    repositories {
        google()
        mavenCentral()
    }
}

rootProject.name = "WheelMakerMobile"
include(":app")

val externalBuildRoot = providers.gradleProperty("wheelmakerBuildRoot")
    .orElse(System.getenv("WHEELMAKER_ANDROID_BUILD_ROOT") ?: "")
    .get()

if (externalBuildRoot.isNotBlank()) {
    val rootBuildDir = file(externalBuildRoot)
    layout.buildDirectory.set(rootBuildDir.resolve("root"))
    gradle.beforeProject { project ->
        val projectName = project.path.removePrefix(":").replace(':', '_').ifBlank { "root" }
        project.layout.buildDirectory.set(rootBuildDir.resolve(projectName))
    }
}
