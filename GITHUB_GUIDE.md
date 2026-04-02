# Guia: Como Subir o Projeto para o GitHub

Este guia explica o passo a passo para enviar o código do **Painel SSH Intel** para um repositório no GitHub.

## 1. Pré-requisitos
* Ter uma conta no [GitHub](https://github.com/).
* Ter o Git instalado no seu servidor ou máquina local.
* Criar um novo repositório vazio no GitHub (não adicione README, licença ou .gitignore ao criar).

## 2. Configuração Inicial do Git
Se você ainda não configurou seu nome e e-mail no Git:
```bash
git config --global user.name "Seu Nome"
git config --global user.email "seu-email@exemplo.com"
```

## 3. Preparando o Repositório Local
No terminal, dentro da pasta do projeto (`c:\Users\Administrator\Documents\painel-ssh`):

1. **Inicialize o Git:**
   ```bash
   git init
   ```

2. **Adicione os arquivos (o .gitignore evitará arquivos pesados):**
   ```bash
   git add .
   ```

3. **Crie o primeiro commit:**
   ```bash
   git commit -m "Initial commit: Painel SSH Intel com persistência de APIs"
   ```

4. **Defina o branch principal:**
   ```bash
   git branch -M main
   ```

## 4. Conectando ao GitHub
Substitua `USUARIO` e `REPOSITORIO` pelos seus dados:

```bash
git remote add origin https://github.com/USUARIO/REPOSITORIO.git
```

## 5. Enviando o Código
```bash
git push -u origin main
```

---

## Dicas Importantes

### Arquivo .gitignore
O projeto já inclui um arquivo `.gitignore` para evitar que arquivos desnecessários sejam enviados, como:
* O banco de dados SQLite (`ssh_panel.db`).
* Binários compilados do Go.
* Arquivos temporários.

### Segurança
**NUNCA** suba o arquivo `ssh_panel.db` se ele contiver senhas de usuários reais. O `.gitignore` configurado protege contra isso.

### Atualizando o Código no GitHub
Sempre que fizer uma mudança e quiser subir a atualização:
```bash
git add .
git commit -m "Descrição da sua alteração"
git push
```
