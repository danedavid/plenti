<script>
    import defaults from '../defaults.js';
    import ButtonWrapper from './button_wrapper.svelte';
    import validateFilename from './validate_filename.js';

    export let showAdd, showEditor;
    let filename = "";

    let selectedType;
    const setType = type => {
        selectedType = type;
    }

    let validationErrors = [];
    const checkFilename = () => {
        validationErrors = validateFilename(filename, selectedType);
        // No errors, redirect to "add" page
        if (validationErrors.length === 0) {
            redirectAndEdit('/#add/' + selectedType + '/' + filename);
        }
    }

    const redirectAndEdit = path => {
        history.pushState(null, '', path);
        showAdd = false; 
        showEditor = true;
    }

</script>

{#if selectedType}
    <h1>Set {selectedType} filename:</h1>
    <div class="filename">
        <span>content/{selectedType}/</span>
        <input placeholder="filename" autofocus bind:value={filename} class="{validationErrors.length > 0 ? 'error' : ''}" />
        <span>.json</span>
    </div>
    {#if validationErrors}
        <ul class="errors">
        {#each validationErrors as error}
            {#if typeof error === "object"}    
                <li>{error.message} <span class="error-link" on:click={() => redirectAndEdit(error.contentPath)}>Edit Content</span>?</li>
            {:else}
                <li>{error}</li>
            {/if}
        {/each}
        </ul>
    {/if}
    <ButtonWrapper>
        <button class="button" on:click={checkFilename}>Set Filename</button>
        <button class="button" on:click={() => setType(null)}>Go back</button>
    </ButtonWrapper>
{:else}
    <h1>Add content of type:</h1>
    <div class="defaults">
        {#each defaults as defaultContent}
            <button on:click={() => setType(defaultContent.type)} class="default">{defaultContent.type}</button>
        {/each}
    </div>
{/if}

<style>
    .defaults {
        display: grid;
        grid-template-columns: 1fr 1fr;
        gap: 10px;
        margin-bottom: 25px;
    }
    .default {
        border-radius: 6px;
        min-height: 50px;
        display: flex;
        align-items: center;
        justify-content: center;
        font-weight: bold;
        cursor: pointer;
        border: 2px solid #1c7fc7;
        color: #1c7fc7;
        background-color: transparent;
    }
    .default:hover {
        background-color: #1c7fc7;
        color: white;
    }
    .filename input {
        background: #ededed;
        border: none;
        border-bottom: 3px solid;
        line-height: 2rem;
        font-size: 1.5rem;
        padding: 0 5px;
        width: 55%;
    }
    input.error {
        background-color: #ffc0c0;
    }
    .button {
        margin: 25px 0;
    }
    .errors {
        color: red;
    }
    .error-link {
        cursor: pointer;
        color: #1c7fc7;
        text-decoration: underline;
    }
</style>