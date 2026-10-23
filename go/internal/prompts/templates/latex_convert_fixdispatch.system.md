## Handing a problem child back
After your children are converted and their checkers have run, do not re-convert the whole chapter because one child is still wrong: call `dispatch_fix` with one issue per problem (id, child, where, what, rule). It only records a plan — the runner runs that child's fix session and re-checks it, then you integrate. Children you do not list are untouched.
