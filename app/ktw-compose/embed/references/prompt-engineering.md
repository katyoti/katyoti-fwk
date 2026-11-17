# Prompt Engineering Knowledge Base

## Six strategies for getting better results

### 1. Write clear instructions

Models can't read your mind. The less the model has to guess at what you want, the more likely you'll get it.

Tactics:
- Include details in your query to get more relevant answers
- Ask the model to adopt a persona
- Use delimiters to clearly indicate distinct parts of the input (triple quotes, XML tags, section titles)
- Specify the steps required to complete a task
- Provide examples (few-shot prompting)
- Specify the desired length of the output (paragraphs, bullet points, word count)

### 2. Provide reference text

Language models can invent fake answers. Providing reference text helps in answering with fewer fabrications.

Tactics:
- Instruct the model to answer using a reference text
- Instruct the model to answer with citations from a reference text

### 3. Split complex tasks into simpler subtasks

Complex tasks have higher error rates. Re-define as a workflow of simpler tasks where outputs of earlier tasks become inputs to later tasks.

Tactics:
- Use intent classification to identify the most relevant instructions for a user query
- For long conversations, summarize or filter previous dialogue
- Summarize long documents piecewise and construct a full summary recursively

### 4. Give the model time to "think"

Models make more reasoning errors when trying to answer immediately. Asking for a chain of thought helps reason toward correct answers.

Tactics:
- Instruct the model to work out its own solution before rushing to a conclusion
- Use inner monologue or a sequence of queries to hide the model's reasoning process
- Ask the model if it missed anything on previous passes

### 5. Use external tools

Compensate for weaknesses by feeding the model outputs of other tools.

Tactics:
- Use embeddings-based search for efficient knowledge retrieval
- Use code execution for accurate calculations or external API calls
- Give the model access to specific functions

### 6. Test changes systematically

Improving performance is easier if you can measure it. Use evaluation procedures ("evals") with representative, diverse test cases.

Tactics:
- Evaluate model outputs with reference to gold-standard answers
- Use model-based evals for subjective criteria

## Key principles

- If outputs are too long, ask for brief replies
- If outputs are too simple, ask for expert-level writing
- If you dislike the format, demonstrate the format you'd like to see
- Providing general instructions is more efficient than demonstrating all permutations by example
- Instructing models to generate a specific number of paragraphs or bullet points is more reliable than targeting exact word counts
- For straightforward tasks, delimiters may not matter; for complex tasks, they are essential to disambiguate
