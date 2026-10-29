using MessagePack;

namespace Presence;

[MessagePackObject]
public class Odd
{
    [Key(0)]
    public bool Live { get; set; }
}
