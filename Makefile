.DEFAULT_GOAL := help

# --- CONFIGURATION ---
MODULE_BASE  := github.com/jupiterclapton/cenackle
GO_VERSION   := 1.24.11
COMPOSE_FILE := compose.yaml
PROTO_DIR    := proto
GEN_DIR      := gen
SERVICES_DIR := services
PKG_DIR      := pkg

# Liste des services (noms des dossiers dans services/)
SERVICES := identity-service social-service feed-service graph-service media-service chat-service gateway

# Choix de l'outil de génération : "buf" (Recommandé) ou "protoc" (Legacy)
PROTO_TOOL ?= buf

GO := go

# --- AIDE ---
.PHONY: help
help: ## Affiche l'aide
	@grep -E '^[a-zA-Z_-]+:.*?## .*$$' $(MAKEFILE_LIST) | sort | awk 'BEGIN {FS = ":.*?## "}; {printf "\033[36m%-20s\033[0m %s\n", $$1, $$2}'

# --- INITIALISATION ---

.PHONY: init
init: install-tools workspace-init ## Initialise le projet (Outils + Workspace)

.PHONY: install-tools
install-tools: ## Installe les outils (Protoc plugins + Buf si nécessaire)
	@echo "📥 Installation des outils Go..."
	@$(GO) install google.golang.org/protobuf/cmd/protoc-gen-go@latest
	@$(GO) install google.golang.org/grpc/cmd/protoc-gen-go-grpc@latest
	@$(GO) install github.com/99designs/gqlgen@latest
	@if [ "$(PROTO_TOOL)" = "buf" ] && ! command -v buf >/dev/null; then \
		echo "⚠️  'buf' n'est pas installé. Visitez https://buf.build/docs/installation"; \
	fi

.PHONY: workspace-init
workspace-init: ## Re-crée le fichier go.work dynamiquement
	@echo "🔥 Reset du go.work..."
	@rm -f go.work
	@echo "🔗 Initialisation du Workspace..."
	@$(GO) work init
	@echo "➕ Ajout du dossier GEN..."
	@$(GO) work use ./$(GEN_DIR)
	@echo "➕ Ajout du dossier PKG..."
	@if [ -d "$(PKG_DIR)" ]; then $(GO) work use ./$(PKG_DIR); fi
	@echo "➕ Ajout des services..."
	@for service in $(SERVICES); do \
		if [ -d "$(SERVICES_DIR)/$$service" ]; then \
			echo "   -> $$service"; \
			$(GO) work use ./$(SERVICES_DIR)/$$service; \
		fi \
	done

# --- GÉNÉRATION DE CODE ---

.PHONY: gen-all
gen-all: gen-proto gen-gateway tidy ## Génère TOUT (Proto + GraphQL) et nettoie

.PHONY: gen-proto
gen-proto: clean-gen ## Génère le code gRPC (selon la variable PROTO_TOOL)
	@echo "⚙️  Génération gRPC avec $(PROTO_TOOL)..."
	@mkdir -p $(GEN_DIR)
ifeq ($(PROTO_TOOL),buf)
	@# CORRECTION : On spécifie $(PROTO_DIR) comme argument.
	@# Buf comprendra que la racine est "proto/" et générera "identity/v1" directement.
	@buf generate $(PROTO_DIR)
else
	@# CORRECTION : On se déplace DANS le dossier proto avant de lancer la boucle.
	@# Ainsi, protoc voit "identity/v1/..." au lieu de "proto/identity/v1/..."
	@cd $(PROTO_DIR) && for file in $$(find . -name '*.proto'); do \
		echo "   -> $$file"; \
		protoc \
			--proto_path=. \
			--go_out=../$(GEN_DIR) --go_opt=paths=source_relative \
			--go-grpc_out=../$(GEN_DIR) --go-grpc_opt=paths=source_relative \
			$$file; \
	done
endif
	@echo "📦 Initialisation du module '$(GEN_DIR)'..."
	@if [ ! -f "$(GEN_DIR)/go.mod" ]; then \
		cd $(GEN_DIR) && $(GO) mod init $(MODULE_BASE)/$(GEN_DIR); \
	fi
	@cd $(GEN_DIR) && $(GO) mod tidy
	@echo "✅ Génération Proto terminée."

.PHONY: gen-gateway
gen-gateway: ## Génère le code GraphQL (gqlgen)
	@echo "🔮 Génération des fichiers GraphQL..."
	@if [ -d "$(SERVICES_DIR)/api-gateway" ]; then \
		cd $(SERVICES_DIR)/api-gateway && $(GO) run github.com/99designs/gqlgen generate; \
	else \
		echo "⚠️  Dossier api-gateway introuvable."; \
	fi

.PHONY: clean-gen
clean-gen: ## Supprime le contenu de gen/ (sauf go.mod si on veut le garder, ici on wipe tout)
	@rm -rf $(GEN_DIR)/*

# --- DÉPENDANCES ---

.PHONY: tidy
tidy: ## Met à jour les dépendances (go mod tidy) partout
	@echo "🧹 Tidy sur GEN..."
	@cd $(GEN_DIR) && $(GO) mod tidy
	@if [ -d "$(PKG_DIR)" ]; then echo "🧹 Tidy sur PKG..."; cd $(PKG_DIR) && $(GO) mod tidy; fi
	@echo "🧹 Tidy sur les services..."
	@for service in $(SERVICES); do \
		if [ -d "$(SERVICES_DIR)/$$service" ]; then \
			echo "   -> $$service"; \
			(cd $(SERVICES_DIR)/$$service && $(GO) mod tidy); \
		fi \
	done
	@echo "🔄 Sync du Workspace..."
	@$(GO) work sync

# --- INFRASTRUCTURE & RUN ---

.PHONY: up
up: ## Lance l'infra Docker (DBs, Broker...)
	@docker compose -f $(COMPOSE_FILE) up -d

.PHONY: down
down: ## Arrête l'infra Docker
	@docker compose -f $(COMPOSE_FILE) down

# Lance un service spécifique, ex: make run-identity-service
run-%:
	@echo "🚀 Lancement de $*..."
	@cd $(SERVICES_DIR)/$* && $(GO) run cmd/main.go

# --- TESTS ---

.PHONY: test
test: ## Lance les tests unitaires sur tout le projet
	@echo "🧪 Test global..."
	@$(GO) test -race ./$(PKG_DIR)/... ./$(SERVICES_DIR)/...

.PHONY: test-services
test-services: ## Teste chaque service individuellement
	@for service in $(SERVICES); do \
		if [ -d "$(SERVICES_DIR)/$$service" ]; then \
			echo "🧪 Test de $$service..."; \
			(cd $(SERVICES_DIR)/$$service && $(GO) test -v ./...); \
		fi \
	done