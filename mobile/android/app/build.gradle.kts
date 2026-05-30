plugins {
    id("com.android.application")
    id("org.jetbrains.kotlin.android")
}

val webAssetsDir = providers.gradleProperty("wheelmakerWebAssetsDir")
    .orElse(System.getenv("WHEELMAKER_ANDROID_WEB_ASSETS") ?: "")
    .get()

android {
    namespace = "com.wheelmaker.android"
    compileSdk = 36

    compileOptions {
        sourceCompatibility = JavaVersion.VERSION_17
        targetCompatibility = JavaVersion.VERSION_17
    }

    defaultConfig {
        applicationId = "com.wheelmaker.android"
        minSdk = 23
        targetSdk = 36
        versionCode = 1
        versionName = "0.0.1"
        testInstrumentationRunner = "androidx.test.runner.AndroidJUnitRunner"
    }

    sourceSets {
        getByName("main") {
            if (webAssetsDir.isNotBlank()) {
                assets.srcDir(webAssetsDir)
            }
        }
    }

    testOptions {
        unitTests.isIncludeAndroidResources = true
    }

    buildTypes {
        getByName("release") {
            signingConfig = signingConfigs.getByName("debug")
            isMinifyEnabled = false
        }
    }
}

kotlin {
    jvmToolchain(17)
}

dependencies {
    implementation("androidx.core:core-ktx:1.17.0")
    implementation("androidx.webkit:webkit:1.15.0")
    implementation("com.squareup.okhttp3:okhttp:4.12.0")
    testImplementation("junit:junit:4.13.2")
    testImplementation("org.json:json:20240303")
}
