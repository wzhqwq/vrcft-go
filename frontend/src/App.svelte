<script lang="ts">
  import logo from './assets/images/logo-universal.png'
</script>

<main>
  <img alt="Wails logo" id="logo" src="{logo}">
  <section aria-labelledby="project-status" class="result" id="result">
    <h1 id="project-status">Project Status</h1>
    <p>Frontend foundation is configured for runtime, plugins, and settings modules.</p>
  </section>
</main>

<style>

  #logo {
    display: block;
    width: 50%;
    height: 50%;
    margin: auto;
    padding: 10% 0 0;
    background-position: center;
    background-repeat: no-repeat;
    background-size: 100% 100%;
    background-origin: content-box;
  }

  .result {
    height: 20px;
    line-height: 20px;
    margin: 1.5rem auto;
  }

</style>
