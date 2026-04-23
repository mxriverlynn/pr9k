 What's done                                              
                                                                                                                                                                            
  In mxriverlynn/homebrew-pr9k:                                                 
  - Seeded with Apache-2.0 LICENSE (mirror of pr9k's) and a README stub that explains the tap is auto-populated by goreleaser. First (and only) commit pushed.              
                                                                                                                                                              
  In mxriverlynn/pr9k on homebrew-installer (PR #149):                                                                                                                      
  - .goreleaser.yml — darwin/linux × arm64/amd64 matrix, builds[].dir: src for the sub-module path, ldflags: -s -w only (no -X version injection), archives bundle the      
  binary + .pr9k/workflow/ with scripts forced to mode 0755, and the brews: block targets the tap using HOMEBREW_TAP_TOKEN.                                                 
  - .github/workflows/release.yml — v* tag trigger, tag-vs-version.Version sanity check runs first (fails early on the "tagged without bumping the const" mistake), then    
  make ci + bundle integration test, then goreleaser.                                                                                                                       
  - README.md and docs/how-to/getting-started.md — brew install is now the primary path, Docker is in the prereqs list, source build demoted to "Building from source",     
  path/to/pr9k/bin/pr9k replaced with plain pr9k.                                                                                                                           
                                                                                                                                                                            
  What you need to do manually                                                                                                                                              
                                                                                                                                                                            
  1. Provision the PAT. Create a fine-grained PAT with Contents: write scoped to only mxriverlynn/homebrew-pr9k, and add it to mxriverlynn/pr9k's repo secrets as           
  HOMEBREW_TAP_TOKEN. Without it, goreleaser's formula-push step will 401 — the GitHub release will still succeed, but the formula won't land on the tap.                   
  2. Merge PR #149 once CI is green and you've reviewed it.                                                                                                                 
  3. Tag and push: git checkout main && git pull && git tag v0.7.1 && git push origin v0.7.1 — that triggers the release workflow.                                          
  4. Smoke test: brew tap mxriverlynn/pr9k && brew install pr9k && pr9k --version should emit pr9k version 0.7.1.                                                           
                                                                                                                                                                            
  I stopped short of tagging because tagging without the PAT produces a half-complete release (tarballs yes, formula no), which is messier to recover from than just waiting
   until the secret is in place.                    
