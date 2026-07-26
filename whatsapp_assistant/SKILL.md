---
name: whatsapp-assistant
description: Read and analyze WhatsApp messages from a local whatsmeow database (e.g., store.db or app.db), identify unanswered chats, and suggest replies based on the user's historical answering style using a generative agent architecture.
---

# WhatsApp Assistant Skill

This skill allows you to act as a contextual AI assistant to manage the user's WhatsApp messages stored in their `whats-gtk` project database. It leverages principles from the "Generative Agents" architecture (Memory Stream, Reflection, and Planning) to provide long-term memory and highly personalized, context-aware replies without relying on a complex external architecture.

## Architecture & Workflow

### 1. Memory Stream (Observation & Ingestion)
- **Locate Database**: The database is likely `store.db` or `app.db` located in `~/workspace/personal/whats-gtk/`.
- **Query Messages**: Use `sqlite3` or a Python script to query the most recent messages.
- **Record Observations**: Extract the chat history as a stream of observational memories. Note who sent the message, the timestamp, and the content. Pay special attention to conversations where the last message was NOT sent by the user (unanswered chats).

### 2. Reflection (Synthesizing User's Style & Context)
- **Analyze User's Past Replies**: Query the database for messages sent by the user in the past to understand their tone, length, and common phrasing.
- **Generate Reflections**: Synthesize high-level insights about the user's communication style (e.g., "The user is typically concise and uses emojis," or "The user takes a formal tone with this specific group"). 
- **Contextualize Relationship**: For each unanswered chat, retrieve past interactions to reflect on the relationship between the user and the sender to accurately gauge the appropriate level of familiarity.

### 3. Planning & Reacting (Suggesting Replies)
- **Determine Action**: Evaluate if an unanswered chat actually requires a response (e.g., a direct question or a new topic) or if it can be ignored (e.g., a simple acknowledgement from the sender).
- **Draft Replies**: For chats needing a response, suggest 2-3 possible replies. These replies must be conditioned on both the current conversation context (Observation) and the user's typical communication style (Reflection).

## Execution Guidelines
- Whenever the user asks you to "check my WhatsApp" or "suggest replies", follow this architecture.
- Do NOT modify the database; perform read-only `SELECT` queries to prevent data corruption.
- Keep the output concise, presenting the unanswered chats and the suggested personalized replies clearly.
