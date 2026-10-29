using MessagePack;

namespace Presence;

[MessagePackObject]
public class Quote
{
    [Key(0)]
    public bool Live { get; set; }
}
