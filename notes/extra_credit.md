## Extra Credit

<ol>
<li> SBOM </li>
<li> Linting/handle warnings </li>
<li> automated tests</li>
<li> Delete/stopping a task was broken at some point </li>
<li> unify and cleanup fmt.printf and log logging </li>
<li> cleanup deprecated function calls </li>
<li> unfuck multiple stats implementations </li>
<li> cleanup unchecked type conversions </li>
<li> adjustable retry task count </li>
<li> Handle sigterm on workers and clean up </li>
<li> podman instead of docker</li>
    <ul>
        <li>Exposed Port mapping doesn't exist with docker sdk</li>
    </ul>
<li> better scheduler </li>
<li> get rid of leaky task/task queue abstraction </li>
<li> dns instead of ip's for nodes? </li>
<li> HA -> is this just implementing etcd?</li>
    - Maybe also keepalived
<li> Mutliple DNS entries at router and RR to start</li>
    - Look at MetalLB later
<li> what would it take to add "service discovery"</li>
</ol>

## VM/Automation Tech

- [bootable containers](https://fedoramagazine.org/get-involved-with-fedora-bootable-containers/)
- Fedora CoreOS/LEAP Micro
- Lima + libvirt