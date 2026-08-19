pipeline {
    agent any

    environment {
        GO_VERSION = '1.25'
        IMAGE_NAME = 'aiops/aiops-server'
        REGISTRY = 'registry.example.com'
        DOCKER_CREDENTIALS = 'docker-registry-credentials'
        GIT_CREDENTIALS = 'git-credentials'
        ARGOCD_URL = 'https://argocd.example.com'
        ARGOCD_TOKEN = credentials('argocd-token')
        GITOPS_REPO = 'git@github.com:org/aiops-gitops.git'
    }

    stages {
        stage('Checkout') {
            steps {
                checkout scm
                script {
                    env.GIT_COMMIT_SHORT = sh(returnStdout: true, script: 'git rev-parse --short HEAD').trim()
                    env.IMAGE_TAG = "${env.BUILD_NUMBER}-${env.GIT_COMMIT_SHORT}"
                }
            }
        }

        stage('Go Test') {
            steps {
                sh 'go test ./... -v -coverprofile=coverage.out'
            }
            post {
                always {
                    junit allowEmptyResults: true, testResults: '**/report.xml'
                }
            }
        }

        stage('Go Vet') {
            steps {
                sh 'go vet ./...'
            }
        }

        stage('Build') {
            steps {
                sh 'CGO_ENABLED=0 GOOS=linux go build -ldflags="-s -w" -o aiops-server ./cmd/server'
            }
        }

        stage('Docker Build') {
            steps {
                script {
                    docker.build("${env.REGISTRY}/${env.IMAGE_NAME}:${env.IMAGE_TAG}")
                }
            }
        }

        stage('Docker Push') {
            steps {
                script {
                    docker.withRegistry("https://${env.REGISTRY}", env.DOCKER_CREDENTIALS) {
                        docker.image("${env.REGISTRY}/${env.IMAGE_NAME}:${env.IMAGE_TAG}").push()
                        docker.image("${env.REGISTRY}/${env.IMAGE_NAME}:${env.IMAGE_TAG}").push('latest')
                    }
                }
            }
        }

        stage('Update GitOps') {
            steps {
                sshagent(credentials: [env.GIT_CREDENTIALS]) {
                    sh '''
                        git clone ${GITOPS_REPO} gitops
                        cd gitops
                        sed -i "s|tag: .*|tag: ${IMAGE_TAG}|" aiops/values.yaml
                        git add aiops/values.yaml
                        git commit -m "chore: update aiops image to ${IMAGE_TAG}"
                        git push
                    '''
                }
            }
        }

        stage('ArgoCD Sync') {
            steps {
                sh '''
                    curl -s -X POST "${ARGOCD_URL}/api/v1/applications/aiops/sync" \
                        -H "Authorization: Bearer ${ARGOCD_TOKEN}" \
                        -H "Content-Type: application/json" \
                        -d '{}'
                '''
            }
        }
    }

    post {
        success {
            echo "Pipeline succeeded! Image: ${env.REGISTRY}/${env.IMAGE_NAME}:${env.IMAGE_TAG}"
        }
        failure {
            echo "Pipeline failed!"
        }
        cleanup {
            sh 'rm -f aiops-server coverage.out'
        }
    }
}
