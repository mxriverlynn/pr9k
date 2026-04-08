# River's Power-Ralph.9000

<img src="./images/power-ralph-9000.jpg">

This is all based on [AI Hero's Getting Started with Ralph](https://www.aihero.dev/getting-started-with-ralph).

## The Loop

`ralph-loop 10` - runs 10 iterations of the loop:

1. Loop Start
   1. Find work ticket
   1. Work it, then update progress.txt and deferred.txt
   1. Write tests for it, then update progress.txt and deferred.txt
   1. Code Review it, then update progress.txt and deferred.txt
1. Repeat loop until iterations complete or no work left
1. Read progress.txt and codify lessons learned
1. Read deferred.txt and create or update issue tickets w/ `deferred` label
