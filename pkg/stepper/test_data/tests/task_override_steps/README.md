## Task inline individual steps and override

This example shows how to reuse individual steps from a Task which can then be interleaved with custom tasks - plus each step can have its properties changed.


* adding an extra **MY_VAR** environment variable to the **write-digest** step
* adding an extra step **my-custom-step** before the reused **digest-to-results** step