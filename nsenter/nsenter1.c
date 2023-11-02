#define _GNU_SOURCE
#include <sched.h>
#include <stdio.h>
#include <stdlib.h>
#include <unistd.h>
#include <sys/types.h>
#include <sys/stat.h>
#include <fcntl.h>

extern char **environ;

/**
 * Execute a command and get the result.
 *
 * @param   cmd - The system command to run.
 * @return  The string command line output of the command.
 */
char *get_output(char *cmd, char *output)
{

    FILE *stream;
    const int max_buffer = 256;
    char buffer[max_buffer];
    stream = popen(cmd, "r");
    if (stream)
    {
        while (!feof(stream))
            if (fgets(buffer, max_buffer, stream) != NULL)
            {
                sprintf(output, "%s", buffer);
                return output;
            }
        pclose(stream);
    }

    return NULL;
}

char *get_true_path(char *version, char *path)
{
    char cmd[1024];
    sprintf(cmd, "/usr/bin/find /var/lib/docker/overlay2 -name '%s' -print -quit", version);
    return get_output(cmd, path);
}

char *trimwhitespace(char *str)
{
    char *end;

    // Trim leading space
    while (isspace((unsigned char)*str))
        str++;

    if (*str == 0) // All spaces?
        return str;

    // Trim trailing space
    end = str + strlen(str) - 1;
    while (end > str && isspace((unsigned char)*end))
        end--;

    // Write new null terminator character
    end[1] = '\0';

    return str;
}

int main(int argc, char **argv)
{
    char *v = getenv("PUSH_VERSION");
    char *shell = "/bin/sh";
    char *def[] = {shell, NULL};
    char *cmd = shell;
    int fdm = open("/proc/1/ns/mnt", O_RDONLY);
    int fdu = open("/proc/1/ns/uts", O_RDONLY);
    int fdn = open("/proc/1/ns/net", O_RDONLY);
    int fdi = open("/proc/1/ns/ipc", O_RDONLY);
    int froot = open("/proc/1/root", O_RDONLY);

    if (fdm == -1 || fdu == -1 || fdn == -1 || fdi == -1 || froot == -1)
    {
        fprintf(stderr, "Failed to open /proc/1 files, are you root?\n");
        exit(1);
    }

    if (setns(fdm, 0) == -1)
    {
        perror("setns:mnt");
        exit(1);
    }
    if (setns(fdu, 0) == -1)
    {
        perror("setns:uts");
        exit(1);
    }
    if (setns(fdn, 0) == -1)
    {
        perror("setns:net");
        exit(1);
    }
    if (setns(fdi, 0) == -1)
    {
        perror("setns:ipc");
        exit(1);
    }
    if (fchdir(froot) == -1)
    {
        perror("fchdir");
        exit(1);
    }
    if (chroot(".") == -1)
    {
        perror("chroot");
        exit(1);
    }

    char new_cmd[10024];
    get_true_path(v, new_cmd);
    char *arguments[20];
    for (int i = 0; i < 20; i++)
    {
        arguments[i] = NULL;
    }

    if (argc > 1)
    {
        for (int i = 0; i < argc; i++)
        {
            printf("%s\n", argv[i]);
            arguments[i] = argv[i];
        }
    }

    if (execve(trimwhitespace(new_cmd), arguments, environ) == -1)
    {
        perror("execve");
        exit(1);
    }
    exit(0);
}